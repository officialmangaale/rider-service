package service

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/push"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// Customer <-> assigned rider chat, one conversation per delivery.
//
// Authority, in order of precedence:
//   - the restaurant order says who the customer is;
//   - the delivery says which rider holds it RIGHT NOW;
//   - a message is stored with the rider it was exchanged with, so history
//     survives a reassignment and a rider never reads another rider's thread.
//
// The apps hide or disable the button, but nothing here trusts that: every
// call re-derives the caller's role from the order.

// Chat roles.
const (
	ChatRoleCustomer = "customer"
	ChatRoleRider    = "rider"
)

// Error codes returned to the apps.
const (
	ChatErrNotFound    = "CHAT_NOT_FOUND"
	ChatErrForbidden   = "CHAT_FORBIDDEN"
	ChatErrInvalid     = "CHAT_INVALID_MESSAGE"
	ChatErrNoRider     = "CHAT_NO_RIDER"
	ChatErrClosed      = "CHAT_CLOSED"
	ChatErrRateLimited = "CHAT_RATE_LIMITED"
	ChatErrUnavailable = "CHAT_UNAVAILABLE"
)

// ChatError is a refused chat operation.
type ChatError struct {
	Code    string
	Message string
}

func (e *ChatError) Error() string { return e.Code + ": " + e.Message }

const (
	chatMaxBodyRunes   = 500
	chatMaxPerMinute   = 30
	chatPushPreviewMax = 80
)

var chatClientIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,64}$`)

// chatActiveStatuses are the delivery states in which chat accepts messages.
// Anything else (delivered, cancelled, no rider) is read-only.
var chatActiveStatuses = map[string]bool{
	"rider_assigned":           true,
	"rider_arrived_restaurant": true,
	"picked_up":                true,
	"on_the_way":               true,
}

// ChatState tells a client what it may do now.
type ChatState struct {
	// CanSend is true when a message would be accepted.
	CanSend bool `json:"can_send"`
	// ReadOnly is true when history exists but sending is closed.
	ReadOnly bool `json:"read_only"`
	// Reason explains !CanSend: "no_rider", "closed" or "not_assigned".
	Reason         string `json:"reason,omitempty"`
	RiderAssigned  bool   `json:"rider_assigned"`
	DeliveryStatus string `json:"delivery_status"`
	Role           string `json:"role"`
	UnreadCount    int    `json:"unread_count"`
}

// ChatMessageDTO is the message shape the apps receive.
type ChatMessageDTO struct {
	ID              int64     `json:"id"`
	OrderID         int       `json:"order_id"`
	SenderRole      string    `json:"sender_role"`
	Body            string    `json:"body"`
	ClientMessageID string    `json:"client_message_id"`
	CreatedAt       time.Time `json:"created_at"`
	Read            bool      `json:"read"`
}

// ChatRealtime is the part of the websocket hub chat needs.
type ChatRealtime interface {
	SendToRider(riderID string, msg ws.WSMessage)
	SendToOrder(orderKey string, msg ws.WSMessage)
}

// ChatPusher delivers a background push to a rider's devices.
type ChatPusher interface {
	ChatMessage(riderID string, notice push.ChatNotice)
}

// ChatOrderKey is the hub channel key customers' chat sockets register under.
// It is distinct from the order-tracking channel so a chat message is never
// delivered to someone who only has tracking access (the restaurant owner).
func ChatOrderKey(orderID int) string { return "chat:" + strconv.Itoa(orderID) }

// chatStore is the persistence ChatService needs. repository.ChatRepository is
// the production implementation.
type chatStore interface {
	ResolveContext(ctx context.Context, orderID int) (repository.ChatContext, error)
	Insert(ctx context.Context, m repository.ChatMessage) (repository.ChatMessage, bool, error)
	List(ctx context.Context, orderID int, riderScope string, afterID int64, limit int) ([]repository.ChatMessage, error)
	MarkRead(ctx context.Context, orderID int, viewerRole, riderScope string, upToID int64) (int64, error)
	Unread(ctx context.Context, orderID int, viewerRole, riderScope string) (int, error)
	HasRiderHistory(ctx context.Context, orderID int, riderID string) (bool, error)
}

type ChatService struct {
	repo     chatStore
	realtime ChatRealtime
	pusher   ChatPusher
	now      func() time.Time

	mu     sync.Mutex
	recent map[string][]time.Time
}

func NewChatService(repo chatStore, realtime ChatRealtime, pusher ChatPusher) *ChatService {
	return &ChatService{
		repo:     repo,
		realtime: realtime,
		pusher:   pusher,
		now:      time.Now,
		recent:   make(map[string][]time.Time),
	}
}

type chatAccess struct {
	role       string
	ctx        repository.ChatContext
	riderScope string // "" for the customer
}

func (s *ChatService) authorize(ctx context.Context, orderID int, userID string) (chatAccess, error) {
	userID = strings.TrimSpace(userID)
	if orderID <= 0 || userID == "" {
		return chatAccess{}, &ChatError{ChatErrForbidden, "chat is not available for this order"}
	}
	c, err := s.repo.ResolveContext(ctx, orderID)
	if errors.Is(err, repository.ErrChatDeliveryNotFound) {
		return chatAccess{}, &ChatError{ChatErrNotFound, "no delivery for this order"}
	}
	if err != nil {
		log.Printf("[CHAT] context lookup failed order_id=%d err=%v", orderID, err)
		return chatAccess{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
	}
	switch {
	case c.IsCustomer(userID):
		return chatAccess{role: ChatRoleCustomer, ctx: c}, nil
	case c.RiderID != "" && c.RiderID == userID:
		return chatAccess{role: ChatRoleRider, ctx: c, riderScope: userID}, nil
	}
	// A rider replaced on this delivery keeps read access to their own thread.
	had, err := s.repo.HasRiderHistory(ctx, orderID, userID)
	if err != nil {
		return chatAccess{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
	}
	if had {
		return chatAccess{role: ChatRoleRider, ctx: c, riderScope: userID}, nil
	}
	return chatAccess{}, &ChatError{ChatErrForbidden, "you are not part of this delivery"}
}

func (a chatAccess) state(unread int) ChatState {
	active := chatActiveStatuses[a.ctx.DeliveryStatus]
	st := ChatState{
		RiderAssigned:  a.ctx.RiderID != "",
		DeliveryStatus: a.ctx.DeliveryStatus,
		Role:           a.role,
		UnreadCount:    unread,
	}
	switch a.role {
	case ChatRoleCustomer:
		switch {
		case a.ctx.RiderID == "":
			st.Reason = "no_rider"
		case !active:
			st.Reason = "closed"
		default:
			st.CanSend = true
		}
	case ChatRoleRider:
		switch {
		case a.ctx.RiderID != a.riderScope:
			st.Reason = "not_assigned"
		case !active:
			st.Reason = "closed"
		default:
			st.CanSend = true
		}
	}
	st.ReadOnly = !st.CanSend && st.Reason != "no_rider"
	return st
}

// Summary is the chat state and unread count, with no message bodies.
func (s *ChatService) Summary(ctx context.Context, orderID int, userID string) (ChatState, error) {
	a, err := s.authorize(ctx, orderID, userID)
	if err != nil {
		return ChatState{}, err
	}
	unread, err := s.repo.Unread(ctx, orderID, a.role, a.riderScope)
	if err != nil {
		return ChatState{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
	}
	return a.state(unread), nil
}

// List returns the caller's visible messages after afterID, plus the state.
func (s *ChatService) List(ctx context.Context, orderID int, userID string, afterID int64) ([]ChatMessageDTO, ChatState, error) {
	a, err := s.authorize(ctx, orderID, userID)
	if err != nil {
		return nil, ChatState{}, err
	}
	rows, err := s.repo.List(ctx, orderID, a.riderScope, afterID, 100)
	if err != nil {
		return nil, ChatState{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
	}
	unread, err := s.repo.Unread(ctx, orderID, a.role, a.riderScope)
	if err != nil {
		return nil, ChatState{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
	}
	out := make([]ChatMessageDTO, 0, len(rows))
	for _, m := range rows {
		out = append(out, toChatDTO(m))
	}
	return out, a.state(unread), nil
}

// MarkRead marks the other party's messages up to upToID as read.
func (s *ChatService) MarkRead(ctx context.Context, orderID int, userID string, upToID int64) (ChatState, error) {
	a, err := s.authorize(ctx, orderID, userID)
	if err != nil {
		return ChatState{}, err
	}
	if upToID > 0 {
		if _, err := s.repo.MarkRead(ctx, orderID, a.role, a.riderScope, upToID); err != nil {
			return ChatState{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
		}
	}
	unread, err := s.repo.Unread(ctx, orderID, a.role, a.riderScope)
	if err != nil {
		return ChatState{}, &ChatError{ChatErrUnavailable, "chat is temporarily unavailable"}
	}
	return a.state(unread), nil
}

// Send stores a message and delivers it. Sending twice with the same
// clientMessageID stores and delivers it once.
func (s *ChatService) Send(ctx context.Context, orderID int, userID, clientMessageID, body string) (ChatMessageDTO, ChatState, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > chatMaxBodyRunes {
		return ChatMessageDTO{}, ChatState{}, &ChatError{ChatErrInvalid, "a message must be 1 to 500 characters"}
	}
	clientMessageID = strings.TrimSpace(clientMessageID)
	if !chatClientIDPattern.MatchString(clientMessageID) {
		return ChatMessageDTO{}, ChatState{}, &ChatError{ChatErrInvalid, "client_message_id is required"}
	}
	a, err := s.authorize(ctx, orderID, userID)
	if err != nil {
		return ChatMessageDTO{}, ChatState{}, err
	}
	if !s.allow(userID) {
		return ChatMessageDTO{}, ChatState{}, &ChatError{ChatErrRateLimited, "you are sending messages too quickly"}
	}
	st := a.state(0)
	if !st.CanSend {
		switch st.Reason {
		case "no_rider":
			return ChatMessageDTO{}, st, &ChatError{ChatErrNoRider, "a delivery partner has not been assigned yet"}
		case "not_assigned":
			return ChatMessageDTO{}, st, &ChatError{ChatErrForbidden, "this delivery is no longer assigned to you"}
		default:
			return ChatMessageDTO{}, st, &ChatError{ChatErrClosed, "this chat is closed"}
		}
	}

	riderID := a.ctx.RiderID
	customerID := userID
	if a.role == ChatRoleRider {
		customerID = firstNonEmptyChat(a.ctx.CustomerIDs...)
	}
	stored, created, err := s.repo.Insert(ctx, repository.ChatMessage{
		OrderID:         orderID,
		DeliveryOrderID: a.ctx.DeliveryOrderID,
		CustomerID:      customerID,
		RiderID:         riderID,
		SenderRole:      a.role,
		SenderID:        userID,
		Body:            body,
		ClientMessageID: clientMessageID,
	})
	if err != nil {
		log.Printf("[CHAT] insert failed order_id=%d err=%v", orderID, err)
		return ChatMessageDTO{}, st, &ChatError{ChatErrUnavailable, "could not send the message"}
	}
	dto := toChatDTO(stored)
	if created {
		s.deliver(a, riderID, dto)
	}
	return dto, st, nil
}

// deliver pushes a stored message to the other party. Delivery is best effort:
// the message is already persisted, and the other side fetches missed messages
// when it reconnects.
func (s *ChatService) deliver(a chatAccess, riderID string, dto ChatMessageDTO) {
	if s.realtime == nil {
		return
	}
	event := ws.WSMessage{Type: "chat.message", Data: dto}
	switch a.role {
	case ChatRoleCustomer:
		if riderID != "" {
			s.realtime.SendToRider(riderID, event)
			if s.pusher != nil {
				s.pusher.ChatMessage(riderID, push.ChatNotice{
					OrderID:   dto.OrderID,
					MessageID: dto.ID,
					Preview:   chatPreview(dto.Body),
				})
			}
		}
	case ChatRoleRider:
		s.realtime.SendToOrder(ChatOrderKey(dto.OrderID), event)
	}
}

// allow is a per-user sliding-window limit, in memory (one instance).
func (s *ChatService) allow(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	cutoff := now.Add(-time.Minute)
	kept := s.recent[userID][:0]
	for _, t := range s.recent[userID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= chatMaxPerMinute {
		s.recent[userID] = kept
		return false
	}
	s.recent[userID] = append(kept, now)
	return true
}

func toChatDTO(m repository.ChatMessage) ChatMessageDTO {
	return ChatMessageDTO{
		ID:              m.MessageID,
		OrderID:         m.OrderID,
		SenderRole:      m.SenderRole,
		Body:            m.Body,
		ClientMessageID: m.ClientMessageID,
		CreatedAt:       m.CreatedAt.UTC(),
		Read:            m.ReadAt != nil,
	}
}

func chatPreview(body string) string {
	runes := []rune(strings.Join(strings.Fields(body), " "))
	if len(runes) > chatPushPreviewMax {
		return string(runes[:chatPushPreviewMax]) + "…"
	}
	return string(runes)
}

func firstNonEmptyChat(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

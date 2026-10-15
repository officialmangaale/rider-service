package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

type memChatStore struct {
	mu       sync.Mutex
	ctx      repository.ChatContext
	messages []repository.ChatMessage
}

func (s *memChatStore) ResolveContext(_ context.Context, orderID int) (repository.ChatContext, error) {
	if orderID != s.ctx.OrderID {
		return repository.ChatContext{}, repository.ErrChatDeliveryNotFound
	}
	return s.ctx, nil
}

func (s *memChatStore) Insert(_ context.Context, m repository.ChatMessage) (repository.ChatMessage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.messages {
		if e.SenderID == m.SenderID && e.ClientMessageID == m.ClientMessageID {
			return e, false, nil
		}
	}
	m.MessageID = int64(len(s.messages) + 1)
	m.CreatedAt = time.Now()
	s.messages = append(s.messages, m)
	return m, true, nil
}

func (s *memChatStore) List(_ context.Context, orderID int, riderScope string, afterID int64, _ int) ([]repository.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []repository.ChatMessage{}
	for _, m := range s.messages {
		if m.MessageID > afterID && (riderScope == "" || m.RiderID == riderScope) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *memChatStore) MarkRead(context.Context, int, string, string, int64) (int64, error) {
	return 0, nil
}
func (s *memChatStore) Unread(context.Context, int, string, string) (int, error) { return 0, nil }
func (s *memChatStore) HasRiderHistory(_ context.Context, _ int, riderID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.messages {
		if m.RiderID == riderID {
			return true, nil
		}
	}
	return false, nil
}

// chatEngine mounts the chat routes the way router.go does, next to the
// existing /delivery routes, with the caller's identity taken from a header
// (the real AuthMiddleware sets the same context key from the token).
func chatEngine(t *testing.T, store *memChatStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewChatHandler(service.NewChatService(store, nil, nil))
	r := gin.New()
	asUser := func(c *gin.Context) {
		c.Set("user_id", c.GetHeader("X-Test-User"))
		c.Next()
	}
	// The pre-existing routes this must coexist with.
	r.POST("/api/v1/delivery/:id/picked-up", func(c *gin.Context) {})
	r.GET("/api/v1/delivery/orders/:orderId/tracking", func(c *gin.Context) {})
	chat := r.Group("/api/v1/delivery/orders/:orderId/chat", asUser)
	chat.GET("", h.Summary)
	chat.GET("/messages", h.List)
	chat.POST("/messages", h.Send)
	chat.POST("/read", h.Read)
	return r
}

func chatCall(t *testing.T, r *gin.Engine, method, path, user, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func newChatStore(riderID, status string) *memChatStore {
	return &memChatStore{ctx: repository.ChatContext{
		OrderID: 16303, DeliveryOrderID: 1, DeliveryStatus: status,
		CustomerIDs: []string{"cust-1"}, RiderID: riderID,
	}}
}

const chatBase = "/api/v1/delivery/orders/16303/chat"

func TestChatRoutesCoexistWithExistingDeliveryRoutes(t *testing.T) {
	// gin panics at registration on a route conflict; reaching here is the test.
	r := chatEngine(t, newChatStore("rider-1", "on_the_way"))
	if code, _ := chatCall(t, r, http.MethodGet, chatBase, "cust-1", ""); code != http.StatusOK {
		t.Fatalf("summary status %d", code)
	}
}

func TestChatHTTPSendAndFetch(t *testing.T) {
	r := chatEngine(t, newChatStore("rider-1", "on_the_way"))

	code, body := chatCall(t, r, http.MethodPost, chatBase+"/messages", "cust-1",
		`{"body":"Please come to Gate 2","client_message_id":"client-msg-0001"}`)
	if code != http.StatusOK {
		t.Fatalf("send status %d: %v", code, body)
	}
	// Retrying the identical request is not an error and not a second message.
	code, _ = chatCall(t, r, http.MethodPost, chatBase+"/messages", "cust-1",
		`{"body":"Please come to Gate 2","client_message_id":"client-msg-0001"}`)
	if code != http.StatusOK {
		t.Fatalf("retry status %d", code)
	}

	code, body = chatCall(t, r, http.MethodGet, chatBase+"/messages", "rider-1", "")
	if code != http.StatusOK {
		t.Fatalf("rider fetch %d", code)
	}
	data := body["data"].(map[string]any)
	messages := data["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected exactly one stored message, got %d", len(messages))
	}
	if first := messages[0].(map[string]any); first["body"] != "Please come to Gate 2" || first["sender_role"] != "customer" {
		t.Fatalf("unexpected message %v", first)
	}
}

func TestChatHTTPAuthorization(t *testing.T) {
	r := chatEngine(t, newChatStore("rider-1", "on_the_way"))
	send := `{"body":"hello","client_message_id":"client-msg-0001"}`

	cases := []struct {
		name, method, path, user, body string
		want                           int
		wantCode                       string
	}{
		{"other customer reads", http.MethodGet, chatBase + "/messages", "cust-2", "", 403, service.ChatErrForbidden},
		{"other customer sends", http.MethodPost, chatBase + "/messages", "cust-2", send, 403, service.ChatErrForbidden},
		{"unassigned rider reads", http.MethodGet, chatBase + "/messages", "rider-9", "", 403, service.ChatErrForbidden},
		{"unassigned rider sends", http.MethodPost, chatBase + "/messages", "rider-9", send, 403, service.ChatErrForbidden},
		{"anonymous", http.MethodGet, chatBase + "/messages", "", "", 403, service.ChatErrForbidden},
		{"unknown order", http.MethodGet, "/api/v1/delivery/orders/1/chat/messages", "cust-1", "", 404, service.ChatErrNotFound},
		{"bad order id", http.MethodGet, "/api/v1/delivery/orders/abc/chat/messages", "cust-1", "", 400, service.ChatErrInvalid},
		{"bad body", http.MethodPost, chatBase + "/messages", "cust-1", `{"body":"","client_message_id":"client-msg-0001"}`, 400, service.ChatErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := chatCall(t, r, tc.method, tc.path, tc.user, tc.body)
			if code != tc.want {
				t.Fatalf("status %d want %d (%v)", code, tc.want, body)
			}
			if body["error_code"] != tc.wantCode {
				t.Fatalf("error_code %v want %s", body["error_code"], tc.wantCode)
			}
		})
	}
}

func TestChatHTTPNoRiderAndClosed(t *testing.T) {
	noRider := chatEngine(t, newChatStore("", "rider_searching"))
	code, body := chatCall(t, noRider, http.MethodPost, chatBase+"/messages", "cust-1",
		`{"body":"hello","client_message_id":"client-msg-0001"}`)
	if code != http.StatusConflict || body["error_code"] != service.ChatErrNoRider {
		t.Fatalf("no rider: %d %v", code, body)
	}

	closed := chatEngine(t, newChatStore("rider-1", "delivered"))
	code, body = chatCall(t, closed, http.MethodPost, chatBase+"/messages", "cust-1",
		`{"body":"hello","client_message_id":"client-msg-0001"}`)
	if code != http.StatusConflict || body["error_code"] != service.ChatErrClosed {
		t.Fatalf("delivered: %d %v", code, body)
	}
	// Reading stays possible once closed.
	if code, _ := chatCall(t, closed, http.MethodGet, chatBase+"/messages", "cust-1", ""); code != http.StatusOK {
		t.Fatalf("history after delivery: %d", code)
	}
}

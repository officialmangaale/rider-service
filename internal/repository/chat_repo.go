package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// ChatContext is the authoritative picture of one delivery for chat:
// who the customer is, which rider currently holds it, and where it is.
type ChatContext struct {
	OrderID         int
	DeliveryOrderID int
	DeliveryStatus  string
	// CustomerIDs are every identifier the order records for the customer
	// (the account id, and the user id kept in order metadata).
	CustomerIDs []string
	// RiderID is the currently assigned rider's user id, "" when none.
	RiderID string
	// RestaurantStatus is the restaurant order's status ("" when unknown).
	RestaurantStatus string
}

// IsCustomer reports whether userID is the customer on this order.
func (c ChatContext) IsCustomer(userID string) bool {
	if userID == "" {
		return false
	}
	for _, id := range c.CustomerIDs {
		if id != "" && id == userID {
			return true
		}
	}
	return false
}

// ChatMessage is one stored message.
type ChatMessage struct {
	MessageID       int64      `json:"message_id"`
	OrderID         int        `json:"order_id"`
	DeliveryOrderID int        `json:"delivery_order_id"`
	CustomerID      string     `json:"customer_id"`
	RiderID         string     `json:"rider_id"`
	SenderRole      string     `json:"sender_role"`
	SenderID        string     `json:"sender_id"`
	Body            string     `json:"body"`
	ClientMessageID string     `json:"client_message_id"`
	CreatedAt       time.Time  `json:"created_at"`
	ReadAt          *time.Time `json:"read_at,omitempty"`
}

// ErrChatDeliveryNotFound means no live delivery exists for the order.
var ErrChatDeliveryNotFound = errors.New("chat: delivery not found")

// ChatRepository reads and writes delivery chat.
type ChatRepository struct {
	db *sql.DB
}

func NewChatRepository(db *sql.DB) *ChatRepository { return &ChatRepository{db: db} }

// ResolveContext reads the delivery and the restaurant order that own the
// conversation. Both are read-only here: chat never changes either.
func (r *ChatRepository) ResolveContext(ctx context.Context, orderID int) (ChatContext, error) {
	var c ChatContext
	var customerID, metaCustomer, customerUUID string
	err := r.db.QueryRowContext(ctx,
		`SELECT d.delivery_order_id, d.order_id, LOWER(TRIM(COALESCE(d.delivery_status, ''))),
		        COALESCE(NULLIF(d.rider_user_id, ''), NULLIF(d.assigned_rider_id, ''), ''),
		        COALESCE(o.customer_id::text, ''),
		        COALESCE(o.metadata->>'customer_user_id', ''),
		        COALESCE(o.customer_uuid::text, ''),
		        LOWER(TRIM(COALESCE(o.order_status::text, '')))
		   FROM delivery_orders d
		   JOIN orders o ON o.order_id = d.order_id
		  WHERE d.order_id = $1 AND d.is_deleted = FALSE AND NOT o.is_deleted
		    AND COALESCE(d.order_type, 'food') = 'food'`,
		orderID,
	).Scan(&c.DeliveryOrderID, &c.OrderID, &c.DeliveryStatus, &c.RiderID,
		&customerID, &metaCustomer, &customerUUID, &c.RestaurantStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ChatContext{}, ErrChatDeliveryNotFound
	}
	if err != nil {
		return ChatContext{}, err
	}
	c.CustomerIDs = []string{customerID, metaCustomer, customerUUID}
	return c, nil
}

// Insert stores a message. It is idempotent on (order, sender, client id): a
// repeat returns the original row with created=false.
func (r *ChatRepository) Insert(ctx context.Context, m ChatMessage) (stored ChatMessage, created bool, err error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO delivery_chat_messages
		   (order_id, delivery_order_id, customer_id, rider_id, sender_role, sender_id, body, client_message_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (order_id, sender_id, client_message_id) DO NOTHING
		 RETURNING message_id, created_at`,
		m.OrderID, m.DeliveryOrderID, m.CustomerID, m.RiderID, m.SenderRole, m.SenderID, m.Body, m.ClientMessageID)
	m2 := m
	switch scanErr := row.Scan(&m2.MessageID, &m2.CreatedAt); {
	case scanErr == nil:
		return m2, true, nil
	case errors.Is(scanErr, sql.ErrNoRows):
		existing, getErr := r.getByClientID(ctx, m.OrderID, m.SenderID, m.ClientMessageID)
		return existing, false, getErr
	default:
		return ChatMessage{}, false, scanErr
	}
}

func (r *ChatRepository) getByClientID(ctx context.Context, orderID int, senderID, clientID string) (ChatMessage, error) {
	var m ChatMessage
	err := r.db.QueryRowContext(ctx,
		`SELECT message_id, order_id, delivery_order_id, customer_id, rider_id, sender_role,
		        sender_id, body, client_message_id, created_at, read_at
		   FROM delivery_chat_messages
		  WHERE order_id=$1 AND sender_id=$2 AND client_message_id=$3`,
		orderID, senderID, clientID,
	).Scan(&m.MessageID, &m.OrderID, &m.DeliveryOrderID, &m.CustomerID, &m.RiderID, &m.SenderRole,
		&m.SenderID, &m.Body, &m.ClientMessageID, &m.CreatedAt, &m.ReadAt)
	return m, err
}

// List returns messages after afterID, oldest first. A rider sees only what was
// exchanged while the delivery was assigned to them; the customer sees all of
// their own order's conversation.
func (r *ChatRepository) List(ctx context.Context, orderID int, riderScope string, afterID int64, limit int) ([]ChatMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query := `SELECT message_id, order_id, delivery_order_id, customer_id, rider_id, sender_role,
	                  sender_id, body, client_message_id, created_at, read_at
	            FROM delivery_chat_messages
	           WHERE order_id = $1 AND message_id > $2`
	args := []any{orderID, afterID}
	if riderScope != "" {
		query += ` AND rider_id = $3`
		args = append(args, riderScope)
	}
	query += ` ORDER BY message_id ASC LIMIT ` + strconv.Itoa(limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.MessageID, &m.OrderID, &m.DeliveryOrderID, &m.CustomerID, &m.RiderID,
			&m.SenderRole, &m.SenderID, &m.Body, &m.ClientMessageID, &m.CreatedAt, &m.ReadAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkRead marks the other party's messages up to upToID as read by viewer.
func (r *ChatRepository) MarkRead(ctx context.Context, orderID int, viewerRole, riderScope string, upToID int64) (int64, error) {
	query := `UPDATE delivery_chat_messages SET read_at = NOW()
	           WHERE order_id = $1 AND message_id <= $2 AND read_at IS NULL AND sender_role <> $3`
	args := []any{orderID, upToID, viewerRole}
	if riderScope != "" {
		query += ` AND rider_id = $4`
		args = append(args, riderScope)
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Unread counts the other party's unread messages visible to the viewer.
func (r *ChatRepository) Unread(ctx context.Context, orderID int, viewerRole, riderScope string) (int, error) {
	query := `SELECT COUNT(*) FROM delivery_chat_messages
	           WHERE order_id = $1 AND read_at IS NULL AND sender_role <> $2`
	args := []any{orderID, viewerRole}
	if riderScope != "" {
		query += ` AND rider_id = $3`
		args = append(args, riderScope)
	}
	var n int
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// HasRiderHistory reports whether riderID exchanged any message on the order,
// which keeps a previously assigned rider able to read their own history.
func (r *ChatRepository) HasRiderHistory(ctx context.Context, orderID int, riderID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM delivery_chat_messages WHERE order_id=$1 AND rider_id=$2)`,
		orderID, riderID).Scan(&ok)
	return ok, err
}

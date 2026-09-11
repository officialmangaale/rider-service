package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
)

// Hub manages all WebSocket connections for riders and customer tracking.
type Hub struct {
	// rider connections: rider_id -> []*Client
	riderClients map[string][]*Client
	// order tracking connections: order_id -> []*Client
	orderClients map[string][]*Client

	mu       sync.RWMutex
	upgrader websocket.Upgrader
}

// Client represents a single WebSocket connection.
type Client struct {
	conn     *websocket.Conn
	hub      *Hub
	channel  string // "rider:{id}" or "order:{id}"
	entityID string // rider_id or order_id
	isRider  bool
	send     chan []byte
	done     chan struct{}

	// Diagnostics only. connID is random and carries no identity; it lets
	// one connection's open, writes and close be matched in the logs.
	connID      string
	openedAt    time.Time
	closeReason atomic.Value // string, a dispatchtrace reason code
}

func newConnID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

func (c *Client) setCloseReason(reason string) {
	c.closeReason.CompareAndSwap(nil, reason)
}

func (c *Client) closeReasonOr(fallback string) string {
	if v, ok := c.closeReason.Load().(string); ok && v != "" {
		return v
	}
	return fallback
}

// WSMessage is the envelope for WebSocket messages.
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		riderClients: make(map[string][]*Client),
		orderClients: make(map[string][]*Client),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
	}
}

// HandleRiderWS handles WebSocket connections for riders.
// Authenticates via JWT token in query param or Authorization header.
func (h *Hub) HandleRiderWS(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract token from query param or header
		tokenStr := c.Query("token")
		if tokenStr == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		// Rejections are logged with a reason code only: never the token,
		// the URL or its query string.
		if tokenStr == "" {
			dispatchtrace.Emit(dispatchtrace.EventConnectionRejected, dispatchtrace.Fields{
				"reason_code": dispatchtrace.ReasonTokenMissing,
			})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
			return
		}

		// Parse JWT
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			dispatchtrace.Emit(dispatchtrace.EventConnectionRejected, dispatchtrace.Fields{
				"reason_code": dispatchtrace.ReasonTokenInvalid,
				"token_error": jwtErrorClass(err),
			})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}
		riderID, _ := claims["sub"].(string)
		if riderID == "" {
			dispatchtrace.Emit(dispatchtrace.EventConnectionRejected, dispatchtrace.Fields{
				"reason_code": dispatchtrace.ReasonMissingSubject,
			})
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing sub"})
			return
		}

		conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[WS] Failed to upgrade rider connection: %v", err)
			dispatchtrace.Emit(dispatchtrace.EventConnectionRejected, dispatchtrace.Fields{
				"reason_code": dispatchtrace.ReasonUpgradeFailed,
				"rider_id":    riderID,
			})
			return
		}

		client := &Client{
			conn:     conn,
			hub:      h,
			channel:  "rider:" + riderID,
			entityID: riderID,
			isRider:  true,
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
			connID:   newConnID(),
			openedAt: time.Now(),
		}

		h.addRiderClient(riderID, client)
		log.Printf("[WS] Rider %s connected", riderID)
		dispatchtrace.Emit(dispatchtrace.EventConnectionOpened, dispatchtrace.Fields{
			"rider_id":          riderID,
			"conn_id":           client.connID,
			"rider_connections": h.RiderConnectionCount(riderID),
		})

		go client.writePump()
		go client.readPump()
	}
}

// HandleOrderTrackingWS handles WebSocket connections for customer order tracking.
func (h *Hub) HandleOrderTrackingWS() gin.HandlerFunc {
	return func(c *gin.Context) {
		orderID := c.Param("orderId")
		if orderID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "order_id required"})
			return
		}

		conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[WS] Failed to upgrade tracking connection: %v", err)
			return
		}

		client := &Client{
			conn:     conn,
			hub:      h,
			channel:  "order:" + orderID,
			entityID: orderID,
			isRider:  false,
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
		}

		h.addOrderClient(orderID, client)
		log.Printf("[WS] Customer tracking connected for order %s", orderID)

		go client.writePump()
		go client.readPump()
	}
}

// SendToRider sends a message to a specific rider's WebSocket channel.
func (h *Hub) SendToRider(riderID string, msg WSMessage) {
	h.SendToRiderCount(riderID, msg)
}

// SendToRiderCount is SendToRider, reporting how many live connections the
// rider had and how many accepted the message into their send buffer. Zero
// connections means the message reached nobody; the caller decides whether
// that matters (an offer is still readable through the pending-requests
// endpoint). A marshal failure returns (0, 0) and is logged.
func (h *Hub) SendToRiderCount(riderID string, msg WSMessage) (connections, enqueued int) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Failed to marshal rider message: %v", err)
		return 0, 0
	}

	h.mu.RLock()
	clients := append([]*Client(nil), h.riderClients[riderID]...)
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- data:
			enqueued++
		default:
			log.Printf("[WS] Rider %s send buffer full, dropping message", riderID)
		}
	}
	return len(clients), enqueued
}

// RiderConnectionCount is the number of live sockets for a rider.
func (h *Hub) RiderConnectionCount(riderID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.riderClients[riderID])
}

// SendToOrder sends a message to all listeners on an order tracking channel.
func (h *Hub) SendToOrder(orderID string, msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] Failed to marshal order message: %v", err)
		return
	}

	h.mu.RLock()
	clients := h.orderClients[orderID]
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			log.Printf("[WS] Order %s tracking send buffer full, dropping message", orderID)
		}
	}
}

// IsRiderConnected checks if a rider has active WebSocket connections.
func (h *Hub) IsRiderConnected(riderID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.riderClients[riderID]) > 0
}

func (h *Hub) addRiderClient(riderID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.riderClients[riderID] = append(h.riderClients[riderID], client)
}

func (h *Hub) removeRiderClient(riderID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.riderClients[riderID]
	for i, c := range clients {
		if c == client {
			h.riderClients[riderID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(h.riderClients[riderID]) == 0 {
		delete(h.riderClients, riderID)
	}
}

func (h *Hub) addOrderClient(orderID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.orderClients[orderID] = append(h.orderClients[orderID], client)
}

func (h *Hub) removeOrderClient(orderID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.orderClients[orderID]
	for i, c := range clients {
		if c == client {
			h.orderClients[orderID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(h.orderClients[orderID]) == 0 {
		delete(h.orderClients, orderID)
	}
}

// writePump pumps messages from the send channel to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second) // ping interval
	defer func() {
		ticker.Stop()
		c.conn.Close()
		if c.isRider {
			c.hub.removeRiderClient(c.entityID, c)
			log.Printf("[WS] Rider %s disconnected", c.entityID)
			dispatchtrace.Emit(dispatchtrace.EventConnectionClosed, dispatchtrace.Fields{
				"rider_id":    c.entityID,
				"conn_id":     c.connID,
				"lifetime":    time.Since(c.openedAt).Round(100 * time.Millisecond),
				"reason_code": c.closeReasonOr(dispatchtrace.ReasonServerClosed),
			})
		} else {
			c.hub.removeOrderClient(c.entityID, c)
			log.Printf("[WS] Order %s tracking disconnected", c.entityID)
		}
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.reportWriteFailure("message", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.reportWriteFailure("ping", err)
				return
			}
		case <-c.done:
			return
		}
	}
}

// readPump reads messages from the WebSocket connection (mainly for keepalive / close detection).
func (c *Client) readPump() {
	defer func() {
		close(c.done)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WS] Unexpected close on %s: %v", c.channel, err)
			}
			c.setCloseReason(dispatchtrace.ReasonPeerClosed)
			break
		}
	}
}

// reportWriteFailure records why a rider socket write failed. Order-tracking
// sockets keep their existing, quieter behaviour.
func (c *Client) reportWriteFailure(kind string, err error) {
	c.setCloseReason(dispatchtrace.ReasonSocketWriteFailed)
	if !c.isRider {
		return
	}
	dispatchtrace.Emit(dispatchtrace.EventWriteFailed, dispatchtrace.Fields{
		"rider_id":    c.entityID,
		"conn_id":     c.connID,
		"frame":       kind,
		"reason_code": dispatchtrace.ReasonSocketWriteFailed,
		"queue_depth": len(c.send),
		"error":       dispatchtrace.ErrorText(err),
	})
}

// jwtErrorClass names why a token was refused without echoing any of it.
func jwtErrorClass(err error) string {
	switch {
	case err == nil:
		return "invalid"
	case errors.Is(err, jwt.ErrTokenExpired):
		return "expired"
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return "not_valid_yet"
	case errors.Is(err, jwt.ErrTokenSignatureInvalid), errors.Is(err, jwt.ErrSignatureInvalid):
		return "bad_signature"
	case errors.Is(err, jwt.ErrTokenMalformed):
		return "malformed"
	default:
		return "invalid"
	}
}

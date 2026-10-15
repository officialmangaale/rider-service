package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// ChatHandler serves the customer <-> rider chat for one delivery. The same
// routes serve both parties: the service derives the caller's role from the
// order, never from the request.
type ChatHandler struct {
	svc *service.ChatService
}

func NewChatHandler(svc *service.ChatService) *ChatHandler { return &ChatHandler{svc: svc} }

type sendChatRequest struct {
	Body            string `json:"body"`
	ClientMessageID string `json:"client_message_id"`
}

type readChatRequest struct {
	UpToID int64 `json:"up_to_id"`
}

// RequireParticipant is the gate for the chat socket: only the order's
// customer, or a rider with a history on it, may open a connection.
func (h *ChatHandler) RequireParticipant() gin.HandlerFunc {
	return func(c *gin.Context) {
		orderID, ok := chatOrderID(c)
		if !ok {
			c.Abort()
			return
		}
		if _, err := h.svc.Summary(c.Request.Context(), orderID, middleware.GetUserID(c)); err != nil {
			writeChatError(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}

// List handles GET /api/v1/delivery/orders/:orderId/chat/messages?after=<id>.
func (h *ChatHandler) List(c *gin.Context) {
	orderID, ok := chatOrderID(c)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
	messages, state, err := h.svc.List(c.Request.Context(), orderID, middleware.GetUserID(c), after)
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"messages": messages, "state": state}})
}

// Summary handles GET .../chat — state and unread count, no bodies.
func (h *ChatHandler) Summary(c *gin.Context) {
	orderID, ok := chatOrderID(c)
	if !ok {
		return
	}
	state, err := h.svc.Summary(c.Request.Context(), orderID, middleware.GetUserID(c))
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"state": state}})
}

// Send handles POST .../chat/messages.
func (h *ChatHandler) Send(c *gin.Context) {
	orderID, ok := chatOrderID(c)
	if !ok {
		return
	}
	var req sendChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, chatFail(service.ChatErrInvalid, "invalid request"))
		return
	}
	message, state, err := h.svc.Send(c.Request.Context(), orderID, middleware.GetUserID(c), req.ClientMessageID, req.Body)
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": message, "state": state}})
}

// Read handles POST .../chat/read.
func (h *ChatHandler) Read(c *gin.Context) {
	orderID, ok := chatOrderID(c)
	if !ok {
		return
	}
	var req readChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, chatFail(service.ChatErrInvalid, "invalid request"))
		return
	}
	state, err := h.svc.MarkRead(c.Request.Context(), orderID, middleware.GetUserID(c), req.UpToID)
	if err != nil {
		writeChatError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"state": state}})
}

func chatOrderID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("orderId"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, chatFail(service.ChatErrInvalid, "invalid order id"))
		return 0, false
	}
	return id, true
}

func writeChatError(c *gin.Context, err error) {
	var chatErr *service.ChatError
	if !errors.As(err, &chatErr) {
		c.JSON(http.StatusServiceUnavailable, chatFail(service.ChatErrUnavailable, "chat is temporarily unavailable"))
		return
	}
	status := http.StatusBadRequest
	switch chatErr.Code {
	case service.ChatErrForbidden:
		status = http.StatusForbidden
	case service.ChatErrNotFound:
		status = http.StatusNotFound
	case service.ChatErrNoRider, service.ChatErrClosed:
		status = http.StatusConflict
	case service.ChatErrRateLimited:
		status = http.StatusTooManyRequests
	case service.ChatErrUnavailable:
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, chatFail(chatErr.Code, chatErr.Message))
}

// chatFail is the error body. `code` is what the customer app reads and
// `error_code` is what the rider app reads; both carry the same value.
func chatFail(code, message string) gin.H {
	return gin.H{"success": false, "message": message, "code": code, "error_code": code}
}

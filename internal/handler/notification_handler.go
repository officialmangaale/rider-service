package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// NotificationHandler handles notification endpoints.
type NotificationHandler struct {
	notifSvc *service.NotificationService
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(notifSvc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notifSvc: notifSvc}
}

// RegisterDeviceToken registers/updates a push notification token.
func (h *NotificationHandler) RegisterDeviceToken(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req dto.DeviceTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "platform and push_token are required")
		return
	}

	if err := h.notifSvc.RegisterDeviceToken(c.Request.Context(), userID, req.Platform, req.PushToken); err != nil {
		dto.InternalError(c, "Failed to register device token")
		return
	}
	dto.Success(c, http.StatusOK, "device token registered", nil)
}

// ListNotifications returns paginated notifications.
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	notifs, total, err := h.notifSvc.List(c.Request.Context(), userID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch notifications")
		return
	}
	dto.Paginated(c, notifs, pq.Page, pq.Limit, total)
}

// MarkRead marks a single notification as read.
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	notifID := c.Param("id")
	if err := h.notifSvc.MarkRead(c.Request.Context(), notifID, userID); err != nil {
		dto.InternalError(c, "Failed to mark notification as read")
		return
	}
	dto.Success(c, http.StatusOK, "notification marked as read", nil)
}

// MarkAllRead marks all notifications as read.
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.notifSvc.MarkAllRead(c.Request.Context(), userID); err != nil {
		dto.InternalError(c, "Failed to mark all notifications as read")
		return
	}
	dto.Success(c, http.StatusOK, "all notifications marked as read", nil)
}

// GetUnreadCount returns the unread notification count.
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	userID := middleware.GetUserID(c)
	count, err := h.notifSvc.GetUnreadCount(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to get unread count")
		return
	}
	dto.Success(c, http.StatusOK, "unread count", gin.H{"unread_count": count})
}

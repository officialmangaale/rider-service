package service

import (
	"context"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// NotificationService handles rider notification operations.
type NotificationService struct {
	notifRepo *repository.NotificationRepository
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(notifRepo *repository.NotificationRepository) *NotificationService {
	return &NotificationService{notifRepo: notifRepo}
}

// RegisterDeviceToken registers or updates a push notification token.
func (s *NotificationService) RegisterDeviceToken(ctx context.Context, riderID, platform, pushToken string) error {
	return s.notifRepo.RegisterDeviceToken(ctx, riderID, platform, pushToken)
}

// List returns paginated notifications.
func (s *NotificationService) List(ctx context.Context, riderID string, limit, offset int) ([]*models.RiderNotification, int64, error) {
	return s.notifRepo.List(ctx, riderID, limit, offset)
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(ctx context.Context, notifID, riderID string) error {
	return s.notifRepo.MarkRead(ctx, notifID, riderID)
}

// MarkAllRead marks all notifications as read.
func (s *NotificationService) MarkAllRead(ctx context.Context, riderID string) error {
	return s.notifRepo.MarkAllRead(ctx, riderID)
}

// GetUnreadCount returns the unread count.
func (s *NotificationService) GetUnreadCount(ctx context.Context, riderID string) (int64, error) {
	return s.notifRepo.GetUnreadCount(ctx, riderID)
}

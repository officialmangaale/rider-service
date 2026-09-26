package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// ErrInvalidDeviceToken is returned for a registration that cannot be a real
// push token, so it is refused instead of stored and retried on every offer.
var ErrInvalidDeviceToken = errors.New("invalid device token registration")

// maxPushTokenLength is generous: FCM registration tokens are about 163 bytes.
const maxPushTokenLength = 4096

// NotificationService handles rider notification operations.
type NotificationService struct {
	notifRepo *repository.NotificationRepository
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(notifRepo *repository.NotificationRepository) *NotificationService {
	return &NotificationService{notifRepo: notifRepo}
}

// RegisterDeviceToken registers or refreshes a push notification token. The
// app calls it at every start and on every FCM token refresh, so repeating it
// is expected and must be harmless.
func (s *NotificationService) RegisterDeviceToken(ctx context.Context, riderID, platform, pushToken string) error {
	platform = strings.ToLower(strings.TrimSpace(platform))
	pushToken = strings.TrimSpace(pushToken)
	if strings.TrimSpace(riderID) == "" ||
		(platform != "android" && platform != "ios") ||
		pushToken == "" || len(pushToken) > maxPushTokenLength ||
		strings.ContainsAny(pushToken, " \t\r\n") {
		return ErrInvalidDeviceToken
	}
	return s.notifRepo.RegisterDeviceToken(ctx, riderID, platform, pushToken)
}

// UnregisterDeviceToken forgets a device token at sign-out, so the next person
// to use the phone does not receive this rider's offers.
func (s *NotificationService) UnregisterDeviceToken(ctx context.Context, riderID, pushToken string) error {
	pushToken = strings.TrimSpace(pushToken)
	if strings.TrimSpace(riderID) == "" || pushToken == "" {
		return ErrInvalidDeviceToken
	}
	return s.notifRepo.RemoveDeviceToken(ctx, riderID, pushToken)
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

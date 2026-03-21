package repository

import (
	"context"
	"database/sql"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// NotificationRepository provides data access for rider_notifications.
type NotificationRepository struct {
	db *sql.DB
}

// NewNotificationRepository creates a new NotificationRepository.
func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// List returns paginated notifications for a rider.
func (r *NotificationRepository) List(ctx context.Context, riderID string, limit, offset int) ([]*models.RiderNotification, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM rider_notifications WHERE rider_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, riderID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, rider_id, title, body, type, data, is_read, created_at
		FROM rider_notifications
		WHERE rider_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, query, riderID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notifs []*models.RiderNotification
	for rows.Next() {
		var n models.RiderNotification
		if err := rows.Scan(&n.ID, &n.RiderID, &n.Title, &n.Body, &n.Type, &n.Data, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		notifs = append(notifs, &n)
	}
	return notifs, total, nil
}

// MarkRead marks a single notification as read.
func (r *NotificationRepository) MarkRead(ctx context.Context, notifID, riderID string) error {
	query := `UPDATE rider_notifications SET is_read = true WHERE id = $1 AND rider_id = $2`
	_, err := r.db.ExecContext(ctx, query, notifID, riderID)
	return err
}

// MarkAllRead marks all notifications as read for a rider.
func (r *NotificationRepository) MarkAllRead(ctx context.Context, riderID string) error {
	query := `UPDATE rider_notifications SET is_read = true WHERE rider_id = $1 AND is_read = false`
	_, err := r.db.ExecContext(ctx, query, riderID)
	return err
}

// GetUnreadCount returns the number of unread notifications.
func (r *NotificationRepository) GetUnreadCount(ctx context.Context, riderID string) (int64, error) {
	var count int64
	query := `SELECT COUNT(*) FROM rider_notifications WHERE rider_id = $1 AND is_read = false`
	err := r.db.QueryRowContext(ctx, query, riderID).Scan(&count)
	return count, err
}

// RegisterDeviceToken upserts a device token in notification_devices (shared table).
func (r *NotificationRepository) RegisterDeviceToken(ctx context.Context, riderID, platform, pushToken string) error {
	// Use the shared notification_devices table
	query := `INSERT INTO notification_devices (tenant_id, outlet_id, user_id, platform, push_token, created_at)
		VALUES ('rider', 'rider', $1, $2, $3, NOW())
		ON CONFLICT ON CONSTRAINT notification_devices_pkey DO UPDATE SET push_token = $3`
	_, err := r.db.ExecContext(ctx, query, riderID, platform, pushToken)
	return err
}

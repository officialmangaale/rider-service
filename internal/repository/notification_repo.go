package repository

import (
	"context"
	"database/sql"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/push"
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

// deviceTenant is the tenant_id rider tokens are stored under in the shared
// notification_devices table (restaurant owners use their own tenants).
const deviceTenant = "rider"

// maxDevicesPerRider bounds how many push tokens one rider keeps. A token
// refresh or a reinstall mints a new one; without a bound the old ones pile up
// and every offer is sent to devices that no longer exist.
const maxDevicesPerRider = 5

// RegisterDeviceToken records a rider's push token in notification_devices
// (a table shared with restaurant-service).
//
// It is safe to call on every app start: an existing registration is
// refreshed, not duplicated. It also enforces two ownership rules that the
// table does not:
//
//   - a token belongs to one app install, and an install belongs to whoever
//     signed in last, so the token is released from any other rider. Without
//     this, a phone passed to a second rider would keep ringing for the first
//     rider's offers;
//   - a rider keeps at most maxDevicesPerRider tokens, newest first.
//
// The upsert is written as UPDATE-then-INSERT-DO-NOTHING rather than
// ON CONFLICT ON CONSTRAINT: the table's primary key is a generated UUID, so
// that form never matched, and every repeated registration of the same token
// failed on the (tenant, user, token) unique index instead.
func (r *NotificationRepository) RegisterDeviceToken(ctx context.Context, riderID, platform, pushToken string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notification_devices WHERE tenant_id = $1 AND push_token = $2 AND user_id <> $3`,
		deviceTenant, pushToken, riderID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE notification_devices SET platform = $4, created_at = NOW()
		 WHERE tenant_id = $1 AND user_id = $2 AND push_token = $3`,
		deviceTenant, riderID, pushToken, platform)
	if err != nil {
		return err
	}
	if updated, _ := res.RowsAffected(); updated == 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notification_devices (tenant_id, outlet_id, user_id, platform, push_token, created_at)
			 VALUES ($1, $1, $2, $3, $4, NOW()) ON CONFLICT DO NOTHING`,
			deviceTenant, riderID, platform, pushToken); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM notification_devices WHERE id IN (
		   SELECT id FROM notification_devices WHERE tenant_id = $1 AND user_id = $2
		   ORDER BY created_at DESC OFFSET $3)`,
		deviceTenant, riderID, maxDevicesPerRider); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveDeviceToken forgets one of the rider's tokens: at sign-out, or when FCM
// reports it retired. Removing a token the rider does not have is not an error.
func (r *NotificationRepository) RemoveDeviceToken(ctx context.Context, riderID, pushToken string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM notification_devices WHERE tenant_id = $1 AND user_id = $2 AND push_token = $3`,
		deviceTenant, riderID, pushToken)
	return err
}

// DeviceTokens returns the rider's registered push tokens, newest first.
func (r *NotificationRepository) DeviceTokens(ctx context.Context, riderID string) ([]push.DeviceToken, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT push_token, platform FROM notification_devices
		 WHERE tenant_id = $1 AND user_id = $2 ORDER BY created_at DESC LIMIT $3`,
		deviceTenant, riderID, maxDevicesPerRider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []push.DeviceToken
	for rows.Next() {
		var token push.DeviceToken
		if err := rows.Scan(&token.Token, &token.Platform); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

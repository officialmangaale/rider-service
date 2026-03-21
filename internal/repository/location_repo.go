package repository

import (
	"context"
	"database/sql"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// LocationHistoryRepository provides data access for rider_location_history.
type LocationHistoryRepository struct {
	db *sql.DB
}

// NewLocationHistoryRepository creates a new LocationHistoryRepository.
func NewLocationHistoryRepository(db *sql.DB) *LocationHistoryRepository {
	return &LocationHistoryRepository{db: db}
}

// Record inserts a new location history entry.
func (r *LocationHistoryRepository) Record(ctx context.Context, riderID string, lat, lng float64, heading, speed *float64) error {
	query := `INSERT INTO rider_location_history (rider_id, latitude, longitude, heading, speed)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query, riderID, lat, lng, heading, speed)
	return err
}

// StatusHistoryRepository provides data access for delivery_status_history.
type StatusHistoryRepository struct {
	db *sql.DB
}

// NewStatusHistoryRepository creates a new StatusHistoryRepository.
func NewStatusHistoryRepository(db *sql.DB) *StatusHistoryRepository {
	return &StatusHistoryRepository{db: db}
}

// Record inserts a new status change entry.
func (r *StatusHistoryRepository) Record(ctx context.Context, tx *sql.Tx, orderID int, fromStatus, toStatus string, changedBy string) error {
	query := `INSERT INTO delivery_status_history (order_id, from_status, to_status, changed_by)
		VALUES ($1, $2, $3, $4)`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, orderID, fromStatus, toStatus, changedBy)
	} else {
		_, err = r.db.ExecContext(ctx, query, orderID, fromStatus, toStatus, changedBy)
	}
	return err
}

// GetByOrderID returns all status history entries for an order.
func (r *StatusHistoryRepository) GetByOrderID(ctx context.Context, orderID int) ([]*models.DeliveryStatusHistory, error) {
	query := `SELECT id, order_id, from_status, to_status, changed_by, metadata, created_at
		FROM delivery_status_history WHERE order_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*models.DeliveryStatusHistory
	for rows.Next() {
		var h models.DeliveryStatusHistory
		if err := rows.Scan(&h.ID, &h.OrderID, &h.FromStatus, &h.ToStatus, &h.ChangedBy, &h.Metadata, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, &h)
	}
	return history, nil
}

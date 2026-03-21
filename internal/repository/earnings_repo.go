package repository

import (
	"context"
	"database/sql"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// EarningsRepository provides data access for the rider_earnings table.
type EarningsRepository struct {
	db *sql.DB
}

// NewEarningsRepository creates a new EarningsRepository.
func NewEarningsRepository(db *sql.DB) *EarningsRepository {
	return &EarningsRepository{db: db}
}

// Create inserts a new earnings entry.
func (r *EarningsRepository) Create(ctx context.Context, tx *sql.Tx, riderID string, orderID *int, earningType string, amount float64, description string) error {
	query := `INSERT INTO rider_earnings (rider_id, order_id, type, amount, description) VALUES ($1, $2, $3, $4, $5)`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, riderID, orderID, earningType, amount, description)
	} else {
		_, err = r.db.ExecContext(ctx, query, riderID, orderID, earningType, amount, description)
	}
	return err
}

// GetSummary returns aggregated earnings for today, this week, and this month.
func (r *EarningsRepository) GetSummary(ctx context.Context, riderID string) (*models.EarningsSummary, error) {
	var s models.EarningsSummary
	query := `SELECT
		COALESCE(SUM(CASE WHEN created_at >= CURRENT_DATE THEN amount ELSE 0 END), 0) as today_earnings,
		COALESCE(SUM(CASE WHEN created_at >= date_trunc('week', CURRENT_DATE) THEN amount ELSE 0 END), 0) as week_earnings,
		COALESCE(SUM(CASE WHEN created_at >= date_trunc('month', CURRENT_DATE) THEN amount ELSE 0 END), 0) as month_earnings,
		COUNT(DISTINCT order_id) as total_orders
		FROM rider_earnings
		WHERE rider_id = $1 AND amount > 0`
	err := r.db.QueryRowContext(ctx, query, riderID).Scan(
		&s.TodayEarnings, &s.WeekEarnings, &s.MonthEarnings, &s.TotalOrders,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetHistory returns paginated earnings history.
func (r *EarningsRepository) GetHistory(ctx context.Context, riderID string, limit, offset int) ([]*models.RiderEarning, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM rider_earnings WHERE rider_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, riderID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, rider_id, order_id, type, amount, description, created_at
		FROM rider_earnings
		WHERE rider_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, query, riderID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var earnings []*models.RiderEarning
	for rows.Next() {
		var e models.RiderEarning
		if err := rows.Scan(&e.ID, &e.RiderID, &e.OrderID, &e.Type, &e.Amount, &e.Description, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		earnings = append(earnings, &e)
	}
	return earnings, total, nil
}

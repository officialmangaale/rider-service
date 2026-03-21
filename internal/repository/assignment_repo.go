package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// AssignmentRepository provides data access for the delivery_assignments table.
type AssignmentRepository struct {
	db *sql.DB
}

// NewAssignmentRepository creates a new AssignmentRepository.
func NewAssignmentRepository(db *sql.DB) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

// Create inserts a new delivery assignment.
func (r *AssignmentRepository) Create(ctx context.Context, orderID int, riderID string, expiresAt time.Time) (*models.DeliveryAssignment, error) {
	var a models.DeliveryAssignment
	query := `INSERT INTO delivery_assignments (order_id, rider_id, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (order_id, rider_id) DO NOTHING
		RETURNING id, order_id, rider_id, status, offered_at, responded_at, expires_at, reject_reason`
	err := r.db.QueryRowContext(ctx, query, orderID, riderID, expiresAt).Scan(
		&a.ID, &a.OrderID, &a.RiderID, &a.Status, &a.OfferedAt, &a.RespondedAt, &a.ExpiresAt, &a.RejectReason,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetPendingForRider returns the current pending assignment for a rider.
func (r *AssignmentRepository) GetPendingForRider(ctx context.Context, riderID string) (*models.DeliveryAssignment, error) {
	var a models.DeliveryAssignment
	query := `SELECT id, order_id, rider_id, status, offered_at, responded_at, expires_at, reject_reason
		FROM delivery_assignments
		WHERE rider_id = $1 AND status = 'pending' AND expires_at > NOW()
		ORDER BY offered_at DESC LIMIT 1`
	err := r.db.QueryRowContext(ctx, query, riderID).Scan(
		&a.ID, &a.OrderID, &a.RiderID, &a.Status, &a.OfferedAt, &a.RespondedAt, &a.ExpiresAt, &a.RejectReason,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetByID returns an assignment by ID.
func (r *AssignmentRepository) GetByID(ctx context.Context, assignmentID string) (*models.DeliveryAssignment, error) {
	var a models.DeliveryAssignment
	query := `SELECT id, order_id, rider_id, status, offered_at, responded_at, expires_at, reject_reason
		FROM delivery_assignments WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, assignmentID).Scan(
		&a.ID, &a.OrderID, &a.RiderID, &a.Status, &a.OfferedAt, &a.RespondedAt, &a.ExpiresAt, &a.RejectReason,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Accept marks an assignment as accepted.
func (r *AssignmentRepository) Accept(ctx context.Context, tx *sql.Tx, assignmentID string) error {
	query := `UPDATE delivery_assignments SET status = 'accepted', responded_at = NOW()
		WHERE id = $1 AND status = 'pending'`
	var result sql.Result
	var err error
	if tx != nil {
		result, err = tx.ExecContext(ctx, query, assignmentID)
	} else {
		result, err = r.db.ExecContext(ctx, query, assignmentID)
	}
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Reject marks an assignment as rejected with a reason.
func (r *AssignmentRepository) Reject(ctx context.Context, assignmentID string, reason string) error {
	query := `UPDATE delivery_assignments SET status = 'rejected', responded_at = NOW(), reject_reason = $2
		WHERE id = $1 AND status = 'pending'`
	_, err := r.db.ExecContext(ctx, query, assignmentID, reason)
	return err
}

package repository

import (
	"context"
	"database/sql"
	"errors"
)

// Serialize joining with accepting a platform offer. The profile table is
// optional during rollout; active legacy links still exclude platform work.
func (r *DeliveryRepository) LockPlatformRider(ctx context.Context, tx *sql.Tx, rider string) error {
	var ready bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass('rider_work_profiles') IS NOT NULL`).Scan(&ready); err != nil {
		return err
	}
	if ready {
		if _, err := tx.ExecContext(ctx, `INSERT INTO rider_work_profiles(rider_id) VALUES($1) ON CONFLICT DO NOTHING`, rider); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `SELECT rider_id FROM rider_work_profiles WHERE rider_id=$1 FOR UPDATE`, rider); err != nil {
			return err
		}
	}
	var linked bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM restaurant_riders WHERE rider_user_id=$1 AND is_active AND COALESCE(status,'active')='active')`, rider).Scan(&linked); err != nil {
		return err
	}
	if linked {
		return errors.New("restaurant riders cannot accept platform orders")
	}
	return nil
}

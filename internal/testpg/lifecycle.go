package testpg

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// online097 is a vendored copy of restaurant-service migration 097 (dispatch
// policy functions and the online_delivery_dispatch_config flag). It lives here
// so the rider-service repository's CI does not depend on a sibling checkout.
//
//go:embed testdata/097_online_delivery_dispatch.sql
var online097 string

// CurrentColumns brings the LifecycleSchema tables up to what the current
// code reads: the restaurants table, the users soft-delete flag and the
// restaurant-service `orders` columns used by online dispatch and delivery
// completion (order source, timestamps, rider/customer projection, payment).
//
// Defaults are chosen so a bare `INSERT INTO orders(order_id, restaurant_id,
// order_status)` is an eligible customer_web delivery order.
const CurrentColumns = `
ALTER TABLE users ADD COLUMN is_deleted boolean DEFAULT false;
CREATE TABLE restaurants(restaurant_id bigint PRIMARY KEY, name text, metadata jsonb,
	latitude double precision, longitude double precision, street_address text, user_id uuid);
ALTER TABLE orders ADD COLUMN is_qrunch boolean DEFAULT false,
	ADD COLUMN metadata jsonb DEFAULT '{"order_source":"customer_web"}',
	ADD COLUMN creation_source text, ADD COLUMN dining_session_id bigint, ADD COLUMN counter_id bigint,
	ADD COLUMN created_at timestamptz DEFAULT now(), ADD COLUMN updated_at timestamptz DEFAULT now(),
	ADD COLUMN picked_up_at timestamptz, ADD COLUMN delivered_at timestamptz, ADD COLUMN assigned_at timestamptz,
	ADD COLUMN rider_assigned_at timestamptz, ADD COLUMN rider_name text, ADD COLUMN rider_phone text,
	ADD COLUMN assigned_rider_name text, ADD COLUMN assigned_rider_phone text, ADD COLUMN rider_vehicle_type text,
	ADD COLUMN rider_vehicle_number text, ADD COLUMN delivery_latitude double precision DEFAULT 28.43,
	ADD COLUMN delivery_longitude double precision DEFAULT 77.04, ADD COLUMN delivery_address text DEFAULT 'Private address',
	ADD COLUMN total_amount numeric DEFAULT 250, ADD COLUMN pay_by text DEFAULT 'card',
	ADD COLUMN payment_status text DEFAULT 'pending';
`

// ApplyLifecycle creates the post-acceptance lifecycle tables at their current
// shape: the mirrored tables, the columns above, migration 097 with online
// dispatch ENABLED (tests exercise the enabled path; a test that needs the flag
// off updates it), and rider-service's wallet migrations 010 and 011, which
// delivery completion posts to. Call it right after Open.
func ApplyLifecycle(db *sql.DB) error {
	if _, err := db.Exec(LifecycleSchema + CurrentColumns); err != nil {
		return err
	}
	if _, err := db.Exec(online097); err != nil {
		return fmt.Errorf("migration 097: %w", err)
	}
	if _, err := db.Exec(`UPDATE online_delivery_dispatch_config SET enabled = true`); err != nil {
		return err
	}
	// The wallet migrations belong to this repository: find them relative to this
	// source file so the result does not depend on the calling package's cwd.
	_, self, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(self), "..", "..", "migrations")
	for _, name := range []string{"010_rider_wallet_and_settlements", "011_rider_wallet_transaction_idempotency"} {
		body, err := os.ReadFile(filepath.Join(migrations, name+".up.sql"))
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

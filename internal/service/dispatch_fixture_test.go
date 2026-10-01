package service

import (
	"database/sql"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

// Riders are users rows with uuid ids, and dispatch reads the customer order
// from the shared `orders` table, so these tests need both. Fixed UUIDs keep
// the assertions readable (rNear, rFarther, ...).
const (
	rNear    = "c6b46748-0000-4000-8000-0000000000a1"
	rFarther = "c6b46748-0000-4000-8000-0000000000a2"
	rA       = "c6b46748-0000-4000-8000-0000000000a3"
	rB       = "c6b46748-0000-4000-8000-0000000000a4"
	rFarAway = "c6b46748-0000-4000-8000-0000000000a5"
)

// dispatchDB opens an isolated schema at the current lifecycle shape with online
// dispatch enabled and the test kitchen (restaurant 27) present.
func dispatchDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatalf("lifecycle schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO restaurants VALUES (27,'Test Kitchen','{"phone":"123"}',$1,$2,'Pickup',NULL)`,
		e2ePickupLat, e2ePickupLng); err != nil {
		t.Fatalf("seed restaurant: %v", err)
	}
	return db
}

// seedDispatchRider creates an active rider account, then their availability
// and location the way e2eSeedRider does.
func seedDispatchRider(t *testing.T, db *sql.DB, id string, kmFromPickup float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO users (id, primary_role, first_name, phone) VALUES ($1,'delivery_driver','Rider','9000000000')`, id); err != nil {
		t.Fatalf("seed rider account: %v", err)
	}
	e2eSeedRider(t, db, id, kmFromPickup)
}

// seedPreparingOrder inserts the customer order dispatch will re-read: an
// online (customer_web) delivery order the kitchen is already preparing.
func seedPreparingOrder(t *testing.T, db *sql.DB, orderID int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO orders (order_id, restaurant_id, order_status) VALUES ($1, 27, 'preparing')`, orderID); err != nil {
		t.Fatalf("seed order: %v", err)
	}
}

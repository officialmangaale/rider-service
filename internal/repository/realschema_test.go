package repository

// Rider-service SQL against PRODUCTION'S REAL SCHEMA (schema only): the token
// revocation tables written by user-service and the pricing snapshot the rider
// payout now reads. Skipped unless TEST_REALSCHEMA_URL is set; refuses any
// non-local host.

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

func realRiderDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("TEST_REALSCHEMA_URL")
	if raw == "" {
		t.Skip("TEST_REALSCHEMA_URL not set")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatalf("refusing to run against a non-local database")
	}
	db, err := sql.Open("postgres", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRealSchemaTokenRevocation(t *testing.T) {
	db := realRiderDB(t)
	ctx := context.Background()
	repo := NewTokenRevocationRepo(db)

	var userID string
	if err := db.QueryRow(`INSERT INTO users (primary_role, status) VALUES ('delivery_driver','active') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	// A clean user and token are not revoked.
	if revoked, err := repo.IsRevoked(ctx, "tok-clean-"+userID, userID, now); err != nil || revoked {
		t.Fatalf("clean token: revoked=%v err=%v", revoked, err)
	}

	// A token logged out individually (user-service writes this row).
	logged := "tok-logged-out-" + userID
	if _, err := db.Exec(`INSERT INTO auth_token_revocations (token_hash, user_id, reason, expires_at) VALUES ($1,$2,'logout',NOW() + interval '1 day')`,
		HashAccessToken(logged), userID); err != nil {
		t.Fatal(err)
	}
	if revoked, err := repo.IsRevoked(ctx, logged, userID, now); err != nil || !revoked {
		t.Fatalf("logged-out token: revoked=%v err=%v, want revoked", revoked, err)
	}
	if revoked, _ := repo.IsRevoked(ctx, "tok-clean-"+userID, userID, now); revoked {
		t.Fatal("another token of the same user must not be revoked by a single logout")
	}

	// "Log out everywhere" / account deletion: a cutoff kills every older token
	// but not one issued after it.
	if _, err := db.Exec(`INSERT INTO auth_user_token_cutoffs (user_id, tokens_invalid_before, reason) VALUES ($1, NOW(), 'logout_all')`, userID); err != nil {
		t.Fatal(err)
	}
	if revoked, err := repo.IsRevoked(ctx, "tok-old-"+userID, userID, now.Add(-time.Hour)); err != nil || !revoked {
		t.Fatalf("token older than the cutoff: revoked=%v err=%v, want revoked", revoked, err)
	}
	if revoked, err := repo.IsRevoked(ctx, "tok-new-"+userID, userID, now.Add(time.Hour)); err != nil || revoked {
		t.Fatalf("token issued after the cutoff: revoked=%v err=%v, want valid", revoked, err)
	}
}

func TestRealSchemaRiderPayoutFromPricingSnapshot(t *testing.T) {
	db := realRiderDB(t)
	ctx := context.Background()
	repo := NewDeliveryRepository(db)

	var rest, withSnapshot, withoutSnapshot int64
	if err := db.QueryRow(`INSERT INTO restaurants (name) VALUES ('Local Kitchen') RETURNING restaurant_id`).Scan(&rest); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*int64{&withSnapshot, &withoutSnapshot} {
		if err := db.QueryRow(`INSERT INTO orders (restaurant_id, order_type, order_status) VALUES ($1,'DELIVERY','delivered') RETURNING order_id`, rest).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO order_pricing_snapshots (order_id, restaurant_id, final_payable_cents, rider_cost_cents, breakdown) VALUES ($1,$2,25000,4550,'{}'::jsonb)`,
		withSnapshot, rest); err != nil {
		t.Fatal(err)
	}

	pay := func(orderID int64, orderType string) float64 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		got, err := repo.riderPayoutFor(ctx, tx, &models.DeliveryOrder{OrderID: int(orderID), OrderType: orderType})
		if err != nil {
			t.Fatalf("riderPayoutFor: %v", err)
		}
		return got
	}
	if got := pay(withSnapshot, "food"); got != 45.50 {
		t.Fatalf("payout with a snapshot = %v, want 45.50 from rider_cost_cents", got)
	}
	if got := pay(withoutSnapshot, "food"); got != legacyFlatRiderPayout {
		t.Fatalf("payout without a snapshot = %v, want the legacy %v", got, legacyFlatRiderPayout)
	}
	// A grocery order shares numeric ids with food orders and has no snapshot.
	if got := pay(withSnapshot, "grocery"); got != legacyFlatRiderPayout {
		t.Fatalf("grocery payout = %v, want the legacy %v even when a food order has the same id", got, legacyFlatRiderPayout)
	}
}

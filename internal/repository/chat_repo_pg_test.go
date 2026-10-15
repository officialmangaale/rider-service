package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"
)

// Runs the chat repository against a real Postgres. Skipped unless
// CHAT_TEST_DATABASE_URL points at a scratch database; it creates and drops
// its own tables, so never point it at a shared or production database.
//
//	CHAT_TEST_DATABASE_URL=postgres://scratch@localhost:55432/chat_test?sslmode=disable \
//	  go test ./internal/repository -run TestChatRepositoryPostgres -count=1
func TestChatRepositoryPostgres(t *testing.T) {
	url := os.Getenv("CHAT_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CHAT_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "013_delivery_chat.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	setup := []string{
		`DROP TABLE IF EXISTS delivery_chat_messages, delivery_orders, orders CASCADE`,
		`DROP TYPE IF EXISTS order_status_enum`,
		`CREATE TYPE order_status_enum AS ENUM ('pending','confirmed','preparing','ready','out_for_delivery','delivered','cancelled')`,
		// Column names and types as in restaurant-service 001/008/025.
		`CREATE TABLE orders (
			order_id SERIAL PRIMARY KEY, customer_id INTEGER NOT NULL,
			order_status order_status_enum DEFAULT 'pending',
			metadata JSONB, customer_uuid UUID, is_deleted BOOLEAN NOT NULL DEFAULT FALSE)`,
		// Column names and types as in rider-service 006/007 plus order_type.
		`CREATE TABLE delivery_orders (
			delivery_order_id SERIAL PRIMARY KEY, order_id INTEGER NOT NULL UNIQUE,
			delivery_status VARCHAR(50) NOT NULL DEFAULT 'pending',
			assigned_rider_id VARCHAR(255), rider_user_id VARCHAR(100),
			order_type VARCHAR(20), is_deleted BOOLEAN NOT NULL DEFAULT FALSE)`,
		string(migration),
	}
	for _, stmt := range setup {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("setup failed: %v\n%.200s", err, stmt)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP TABLE IF EXISTS delivery_chat_messages, delivery_orders, orders CASCADE`)
		_, _ = db.Exec(`DROP TYPE IF EXISTS order_status_enum`)
	})

	// Order 16303: customer 77 (uuid + metadata id), rider-1 on it.
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	mustExec(`INSERT INTO orders (order_id, customer_id, order_status, metadata, customer_uuid)
	          VALUES (16303, 77, 'out_for_delivery', '{"customer_user_id":"web-cust-9"}',
	                  '6f1c1c8e-0a53-4a43-9f3e-2f0a3b5a7c11')`)
	mustExec(`INSERT INTO delivery_orders (order_id, delivery_status, rider_user_id, assigned_rider_id)
	          VALUES (16303, 'picked_up', 'rider-1', 'rider-1')`)
	// Order 5: assigned_rider_id only (older rows), no rider_user_id.
	mustExec(`INSERT INTO orders (order_id, customer_id) VALUES (5, 78)`)
	mustExec(`INSERT INTO delivery_orders (order_id, delivery_status, assigned_rider_id)
	          VALUES (5, 'rider_assigned', 'rider-legacy')`)
	// Order 6: unassigned.
	mustExec(`INSERT INTO orders (order_id, customer_id) VALUES (6, 79)`)
	mustExec(`INSERT INTO delivery_orders (order_id, delivery_status) VALUES (6, 'rider_searching')`)
	// Order 7: grocery delivery sharing the numeric id of nothing food — must not resolve.
	mustExec(`INSERT INTO orders (order_id, customer_id) VALUES (7, 80)`)
	mustExec(`INSERT INTO delivery_orders (order_id, delivery_status, rider_user_id, order_type)
	          VALUES (7, 'picked_up', 'rider-g', 'grocery')`)

	repo := NewChatRepository(db)

	t.Run("resolves customer by id, metadata id and uuid", func(t *testing.T) {
		c, err := repo.ResolveContext(ctx, 16303)
		if err != nil {
			t.Fatal(err)
		}
		if c.RiderID != "rider-1" || c.DeliveryStatus != "picked_up" || c.RestaurantStatus != "out_for_delivery" {
			t.Fatalf("unexpected context %+v", c)
		}
		for _, id := range []string{"77", "web-cust-9", "6f1c1c8e-0a53-4a43-9f3e-2f0a3b5a7c11"} {
			if !c.IsCustomer(id) {
				t.Fatalf("%q should be recognised as the customer (%v)", id, c.CustomerIDs)
			}
		}
		if c.IsCustomer("78") || c.IsCustomer("") {
			t.Fatal("another customer or empty id accepted")
		}
	})

	t.Run("falls back to assigned_rider_id and handles no rider", func(t *testing.T) {
		c, err := repo.ResolveContext(ctx, 5)
		if err != nil || c.RiderID != "rider-legacy" {
			t.Fatalf("legacy rider: %+v %v", c, err)
		}
		c, err = repo.ResolveContext(ctx, 6)
		if err != nil || c.RiderID != "" {
			t.Fatalf("unassigned: %+v %v", c, err)
		}
	})

	t.Run("unknown and grocery deliveries do not resolve", func(t *testing.T) {
		if _, err := repo.ResolveContext(ctx, 999); err != ErrChatDeliveryNotFound {
			t.Fatalf("unknown order: %v", err)
		}
		if _, err := repo.ResolveContext(ctx, 7); err != ErrChatDeliveryNotFound {
			t.Fatalf("grocery delivery: %v", err)
		}
	})

	msg := func(role, sender, rider, body, client string) ChatMessage {
		return ChatMessage{OrderID: 16303, DeliveryOrderID: 1, CustomerID: "77", RiderID: rider,
			SenderRole: role, SenderID: sender, Body: body, ClientMessageID: client}
	}

	t.Run("insert is idempotent per sender and client id", func(t *testing.T) {
		first, created, err := repo.Insert(ctx, msg("customer", "77", "rider-1", "Gate 2", "client-0001"))
		if err != nil || !created || first.MessageID == 0 {
			t.Fatalf("first insert: %+v created=%v err=%v", first, created, err)
		}
		again, created, err := repo.Insert(ctx, msg("customer", "77", "rider-1", "Gate 2", "client-0001"))
		if err != nil || created || again.MessageID != first.MessageID {
			t.Fatalf("repeat insert: %+v created=%v err=%v", again, created, err)
		}
		// Same client id from the other party is a different message.
		other, created, err := repo.Insert(ctx, msg("rider", "rider-1", "rider-1", "ok", "client-0001"))
		if err != nil || !created || other.MessageID == first.MessageID {
			t.Fatalf("other sender: %+v created=%v err=%v", other, created, err)
		}
	})

	t.Run("body length is enforced by the database", func(t *testing.T) {
		if _, _, err := repo.Insert(ctx, msg("customer", "77", "rider-1", "", "client-empty1")); err == nil {
			t.Fatal("empty body accepted")
		}
	})

	t.Run("rider scope hides a previous rider's messages", func(t *testing.T) {
		// Reassignment: rider-2 now holds the order; rider-1's history remains.
		if _, _, err := repo.Insert(ctx, msg("customer", "77", "rider-2", "Are you there?", "client-0002")); err != nil {
			t.Fatal(err)
		}
		all, err := repo.List(ctx, 16303, "", 0, 50)
		if err != nil || len(all) != 3 {
			t.Fatalf("customer sees %d messages (%v)", len(all), err)
		}
		forRider2, err := repo.List(ctx, 16303, "rider-2", 0, 50)
		if err != nil || len(forRider2) != 1 || forRider2[0].Body != "Are you there?" {
			t.Fatalf("rider-2 sees %+v (%v)", forRider2, err)
		}
		after, err := repo.List(ctx, 16303, "", all[1].MessageID, 50)
		if err != nil || len(after) != 1 || after[0].MessageID != all[2].MessageID {
			t.Fatalf("after cursor: %+v (%v)", after, err)
		}
		if ok, _ := repo.HasRiderHistory(ctx, 16303, "rider-1"); !ok {
			t.Fatal("rider-1 should keep history")
		}
		if ok, _ := repo.HasRiderHistory(ctx, 16303, "rider-9"); ok {
			t.Fatal("rider-9 never held this order")
		}
	})

	t.Run("unread counts and mark read are per viewer", func(t *testing.T) {
		// Customer's unread = rider-sent messages not yet read.
		n, err := repo.Unread(ctx, 16303, "customer", "")
		if err != nil || n != 1 {
			t.Fatalf("customer unread = %d (%v)", n, err)
		}
		all, _ := repo.List(ctx, 16303, "", 0, 50)
		changed, err := repo.MarkRead(ctx, 16303, "customer", "", all[len(all)-1].MessageID)
		if err != nil || changed != 1 {
			t.Fatalf("mark read changed %d (%v)", changed, err)
		}
		if n, _ := repo.Unread(ctx, 16303, "customer", ""); n != 0 {
			t.Fatalf("customer unread after read = %d", n)
		}
		// rider-2 still has the customer's message unread.
		if n, _ := repo.Unread(ctx, 16303, "rider", "rider-2"); n != 1 {
			t.Fatalf("rider-2 unread = %d", n)
		}
	})
}

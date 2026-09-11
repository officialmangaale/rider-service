// Package testpg gives tests a real PostgreSQL with the dispatch tables.
//
// sqlmock matches SQL by pattern and never executes it, which is how
// FindNearestRiders shipped with a query PostgreSQL rejects on every call.
// Tests that use this package run the real SQL.
//
// Usage: set TEST_DATABASE_URL to an EMPTY database you own, e.g.
//
//	TEST_DATABASE_URL='postgres://tester@/rider_it?host=/tmp/pg&sslmode=disable' go test ./...
//
// Without it, the tests skip. Each test gets its own schema (dropped
// afterwards) with the dispatch tables mirrored from production
// (information_schema, 2026-09-11). The helper refuses to run against a
// database that already contains application tables, so it cannot be pointed
// at production by mistake.
package testpg

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// EnvVar names the connection string for integration tests.
const EnvVar = "TEST_DATABASE_URL"

// Schema mirrors the production dispatch tables: same column types,
// nullability, defaults and constraints.
const Schema = `
CREATE TABLE rider_locations (
	rider_id        varchar(255) PRIMARY KEY,
	latitude        double precision NOT NULL,
	longitude       double precision NOT NULL,
	last_updated_at timestamptz NOT NULL DEFAULT now(),
	is_deleted      boolean NOT NULL DEFAULT false
);
CREATE TABLE rider_availability (
	rider_id         varchar(255) PRIMARY KEY,
	is_online        boolean NOT NULL DEFAULT false,
	is_available     boolean NOT NULL DEFAULT false,
	current_order_id integer,
	updated_at       timestamptz NOT NULL DEFAULT now(),
	is_deleted       boolean NOT NULL DEFAULT false
);
CREATE TABLE delivery_orders (
	delivery_order_id serial PRIMARY KEY,
	order_id          integer NOT NULL UNIQUE,
	restaurant_id     integer NOT NULL,
	customer_id       integer NOT NULL,
	pickup_latitude   double precision NOT NULL,
	pickup_longitude  double precision NOT NULL,
	pickup_address    text NOT NULL DEFAULT '',
	drop_latitude     double precision NOT NULL,
	drop_longitude    double precision NOT NULL,
	drop_address      text NOT NULL DEFAULT '',
	amount            double precision NOT NULL DEFAULT 0,
	payment_mode      varchar(50) NOT NULL DEFAULT 'cod',
	delivery_status   varchar(50) NOT NULL DEFAULT 'pending',
	assigned_rider_id varchar(255),
	created_at        timestamptz NOT NULL DEFAULT now(),
	updated_at        timestamptz NOT NULL DEFAULT now(),
	picked_up_at      timestamptz,
	delivered_at      timestamptz,
	rider_user_id     varchar(100),
	assignment_type   varchar(40) DEFAULT 'platform',
	restaurant_owned  boolean DEFAULT false,
	restaurant_name   varchar(150),
	restaurant_phone  varchar(20),
	assigned_at       timestamp,
	is_deleted        boolean NOT NULL DEFAULT false,
	customer_name     varchar(200) NOT NULL DEFAULT '',
	customer_phone    varchar(30) NOT NULL DEFAULT '',
	items_summary     text NOT NULL DEFAULT '',
	rider_arrived_at  timestamptz
);
CREATE TABLE delivery_order_requests (
	request_id        serial PRIMARY KEY,
	delivery_order_id integer NOT NULL REFERENCES delivery_orders(delivery_order_id),
	order_id          integer NOT NULL,
	rider_id          varchar(255) NOT NULL,
	status            varchar(50) NOT NULL DEFAULT 'pending',
	distance_km       double precision NOT NULL DEFAULT 0,
	expires_at        timestamptz NOT NULL,
	created_at        timestamptz NOT NULL DEFAULT now(),
	updated_at        timestamptz NOT NULL DEFAULT now(),
	is_deleted        boolean NOT NULL DEFAULT false,
	UNIQUE (delivery_order_id, rider_id)
);
CREATE TABLE processed_events (
	event_id     varchar(255) PRIMARY KEY,
	order_id     integer,
	event_type   varchar(100) NOT NULL,
	processed_at timestamptz NOT NULL DEFAULT now(),
	is_deleted   boolean NOT NULL DEFAULT false
);`

// Open returns a pool whose connections all use a fresh schema holding the
// dispatch tables. The schema is dropped when the test ends.
func Open(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv(EnvVar)
	if raw == "" {
		t.Skipf("set %s to an empty PostgreSQL database to run integration tests", EnvVar)
	}

	admin, err := sql.Open("postgres", raw)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Fatalf("connect to %s: %v", EnvVar, err)
	}
	var appTable sql.NullString
	if err := admin.QueryRow(`SELECT COALESCE(to_regclass('public.orders')::text, to_regclass('public.users')::text)`).Scan(&appTable); err != nil {
		t.Fatalf("inspect database: %v", err)
	}
	if appTable.Valid {
		t.Fatalf("refusing to run: %s points at a database with application tables (%s). Use an empty test database.", EnvVar, appTable.String)
	}

	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random: %v", err)
	}
	schema := "rider_it_" + hex.EncodeToString(b)
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("postgres", raw)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	})

	db, err := sql.Open("postgres", withSearchPath(t, raw, schema))
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return db
}

// withSearchPath adds search_path to the connection string; lib/pq sends
// unrecognised parameters to the server as session settings.
func withSearchPath(t *testing.T, raw, schema string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		t.Fatalf("%s must be a postgres:// URL", EnvVar)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

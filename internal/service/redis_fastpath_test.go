package service

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	dispatchcache "github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/cache"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// The Redis dispatch fast path on a real Redis and a real PostgreSQL.
// Needs TEST_DATABASE_URL and TEST_REDIS_URL (an EMPTY, throwaway Redis,
// e.g. unix:///tmp/redis.sock); skipped otherwise. The helper refuses a Redis
// that already holds keys and flushes only what the test wrote.

func openTestRedis(t *testing.T) *dispatchcache.RedisDispatchCache {
	t.Helper()
	raw := os.Getenv("TEST_REDIS_URL")
	if raw == "" {
		t.Skip("set TEST_REDIS_URL to an empty throwaway Redis to run Redis fast-path tests")
	}
	opt, err := redis.ParseURL(raw)
	if err != nil {
		t.Fatalf("TEST_REDIS_URL: %v", err)
	}
	admin := redis.NewClient(opt)
	t.Cleanup(func() { admin.Close() })
	size, err := admin.DBSize(context.Background()).Result()
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	if size != 0 {
		t.Fatalf("refusing to run: TEST_REDIS_URL holds %d keys; use an empty throwaway Redis", size)
	}
	t.Cleanup(func() { admin.FlushDB(context.Background()) })

	cache, err := dispatchcache.NewRedisDispatchCache(raw)
	if err != nil || cache == nil {
		t.Fatalf("NewRedisDispatchCache: %v", err)
	}
	t.Cleanup(func() { cache.Close() })
	return cache
}

func fastPathService(db *sql.DB, cache *dispatchcache.RedisDispatchCache, maxRiders int) *DeliveryService {
	return NewDeliveryService(
		repository.NewDeliveryRepository(db),
		repository.NewRiderRepository(db),
		ws.NewHub(), nil, 5.0, maxRiders, 30, cache,
	)
}

// seedIndexed stores the rider in PostgreSQL and indexes the same location
// in Redis, as both location routes now do.
func seedIndexed(t *testing.T, db *sql.DB, svc *DeliveryService, id string, km float64) {
	t.Helper()
	e2eSeedRider(t, db, id, km)
	svc.IndexRiderLocation(context.Background(), id, e2ePickupLat+km*e2eKm, e2ePickupLng)
}

func offeredRiders(t *testing.T, db *sql.DB, orderID int) map[string]bool {
	t.Helper()
	rows, err := db.Query(`SELECT rider_id FROM delivery_order_requests WHERE order_id = $1`, orderID)
	if err != nil {
		t.Fatalf("offers: %v", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out[id] = true
	}
	return out
}

func TestRedisFastPathIsUsedWhenItFillsTheOfferList(t *testing.T) {
	db := testpg.Open(t)
	cache := openTestRedis(t)
	lines := captureDispatch(t)
	svc := fastPathService(db, cache, 2)
	seedIndexed(t, db, svc, "r-near", 0.3)
	seedIndexed(t, db, svc, "r-mid", 0.8)
	seedIndexed(t, db, svc, "r-far", 2.0)

	if err := svc.ProcessOrderPlacedEvent(context.Background(), confirmedOrderEvent(20001)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	got := offeredRiders(t, db, 20001)
	if len(got) != 2 || !got["r-near"] || !got["r-mid"] {
		t.Fatalf("offered %v, want the nearest two", got)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "event=dispatch.search.redis") ||
		!strings.Contains(strings.Join(*lines, "\n"), "result=used") {
		t.Fatalf("expected the Redis path to be used:\n%s", strings.Join(*lines, "\n"))
	}
}

// Redis still lists a rider who has since taken an order. PostgreSQL decides,
// so that rider is never offered.
func TestStaleRedisEntryNeverProducesAnOffer(t *testing.T) {
	db := testpg.Open(t)
	cache := openTestRedis(t)
	captureDispatch(t)
	svc := fastPathService(db, cache, 5)
	seedIndexed(t, db, svc, "r-free", 0.5)
	seedIndexed(t, db, svc, "r-now-busy", 0.2)
	if _, err := db.Exec(`UPDATE rider_availability SET is_available = false, current_order_id = 999 WHERE rider_id = 'r-now-busy'`); err != nil {
		t.Fatal(err)
	}

	if err := svc.ProcessOrderPlacedEvent(context.Background(), confirmedOrderEvent(20002)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	got := offeredRiders(t, db, 20002)
	if got["r-now-busy"] || !got["r-free"] {
		t.Fatalf("offered %v; a busy rider must never be offered", got)
	}
}

// A rider PostgreSQL knows but Redis does not (failed cache write, flushed
// cache, fresh deploy) is still offered: the SQL search runs.
func TestRiderMissingFromRedisIsStillOffered(t *testing.T) {
	db := testpg.Open(t)
	cache := openTestRedis(t)
	lines := captureDispatch(t)
	svc := fastPathService(db, cache, 5)
	seedIndexed(t, db, svc, "r-indexed", 0.5)
	e2eSeedRider(t, db, "r-not-indexed", 0.4) // PostgreSQL only

	if err := svc.ProcessOrderPlacedEvent(context.Background(), confirmedOrderEvent(20003)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	got := offeredRiders(t, db, 20003)
	if !got["r-indexed"] || !got["r-not-indexed"] {
		t.Fatalf("offered %v, want both riders", got)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "reason_code=redis_too_few_verified") {
		t.Fatalf("expected an SQL fallback with its reason:\n%s", strings.Join(*lines, "\n"))
	}
}

// An empty Redis, as right after deploy, falls back to PostgreSQL.
func TestEmptyRedisFallsBackToPostgres(t *testing.T) {
	db := testpg.Open(t)
	cache := openTestRedis(t)
	lines := captureDispatch(t)
	svc := fastPathService(db, cache, 5)
	e2eSeedRider(t, db, "r-only-in-postgres", 0.5)

	if err := svc.ProcessOrderPlacedEvent(context.Background(), confirmedOrderEvent(20004)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	if got := offeredRiders(t, db, 20004); !got["r-only-in-postgres"] {
		t.Fatalf("offered %v", got)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "reason_code=redis_no_candidates") {
		t.Fatalf("expected redis_no_candidates:\n%s", strings.Join(*lines, "\n"))
	}
}

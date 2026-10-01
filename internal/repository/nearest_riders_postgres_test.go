package repository

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

// These run the real SQL on PostgreSQL (TEST_DATABASE_URL; skipped without
// it). FindNearestRiders once used HAVING without GROUP BY, which PostgreSQL
// rejects on every call; every dispatch failed while the sqlmock tests passed.
//
// A rider is a users row (uuid id, active, rider role) plus availability and
// location rows keyed by that id's text. Tests name riders readably; seed()
// derives a stable UUID per name and ids() maps results back to the names.

const pickupLat, pickupLng = 28.4139, 77.0422

// kmNorth is roughly one kilometre of latitude.
const kmNorth = 1.0 / 111.0

type seedRider struct {
	id           string
	online       bool
	available    bool
	currentOrder *int
	kmFromPickup float64
	ageSeconds   int
	noLocation   bool

	// Account state; the zero values mean an ordinary active rider.
	role    string // default "delivery_driver"
	status  string // default "active"
	deleted bool   // users.is_deleted
}

var riderNames sync.Map // uuid text -> readable test name

// riderUUID is a stable, valid UUID for a readable rider name.
func riderUUID(name string) string {
	s := sha1.Sum([]byte("rider:" + name))
	return fmt.Sprintf("%x-%x-%x-%x-%x", s[0:4], s[4:6], s[6:8], s[8:10], s[10:16])
}

func nameOf(uuid string) string {
	if n, ok := riderNames.Load(uuid); ok {
		return n.(string)
	}
	return uuid
}

// nearestDB opens an isolated schema with the full current lifecycle shape
// (users, dispatch config with online dispatch enabled, and so on).
func nearestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatalf("lifecycle schema: %v", err)
	}
	return db
}

func seed(t *testing.T, db *sql.DB, riders ...seedRider) {
	t.Helper()
	for _, r := range riders {
		id := riderUUID(r.id)
		riderNames.Store(id, r.id)

		role, status := r.role, r.status
		if role == "" {
			role = "delivery_driver"
		}
		if status == "" {
			status = "active"
		}
		if _, err := db.Exec(`INSERT INTO users (id, primary_role, status, is_deleted) VALUES ($1,$2,$3,$4)`,
			id, role, status, r.deleted); err != nil {
			t.Fatalf("seed user %s: %v", r.id, err)
		}
		if _, err := db.Exec(`INSERT INTO rider_availability (rider_id, is_online, is_available, current_order_id) VALUES ($1,$2,$3,$4)`,
			id, r.online, r.available, r.currentOrder); err != nil {
			t.Fatalf("seed availability %s: %v", r.id, err)
		}
		if r.noLocation {
			continue
		}
		if _, err := db.Exec(`INSERT INTO rider_locations (rider_id, latitude, longitude, last_updated_at)
			VALUES ($1, $2, $3, NOW() - make_interval(secs => $4))`,
			id, pickupLat+r.kmFromPickup*kmNorth, pickupLng, r.ageSeconds); err != nil {
			t.Fatalf("seed location %s: %v", r.id, err)
		}
	}
}

func ids(t *testing.T, repo *DeliveryRepository, limit int) []string {
	t.Helper()
	riders, err := repo.FindNearestRiders(context.Background(), pickupLat, pickupLng, 5.0, limit)
	if err != nil {
		t.Fatalf("FindNearestRiders must execute on PostgreSQL: %v", err)
	}
	out := make([]string, len(riders))
	for i, r := range riders {
		out[i] = nameOf(r.RiderID)
	}
	return out
}

func TestFindNearestRidersOnPostgresAppliesEveryRule(t *testing.T) {
	db := nearestDB(t)
	busy := 42
	seed(t, db,
		seedRider{id: "eligible-far", online: true, available: true, kmFromPickup: 3.0, ageSeconds: 30},
		seedRider{id: "eligible-near", online: true, available: true, kmFromPickup: 0.5, ageSeconds: 30},
		seedRider{id: "eligible-4min", online: true, available: true, kmFromPickup: 1.0, ageSeconds: 240},
		seedRider{id: "offline", online: false, available: true, kmFromPickup: 0.2, ageSeconds: 30},
		seedRider{id: "unavailable", online: true, available: false, kmFromPickup: 0.2, ageSeconds: 30},
		seedRider{id: "busy", online: true, available: true, currentOrder: &busy, kmFromPickup: 0.2, ageSeconds: 30},
		seedRider{id: "stale", online: true, available: true, kmFromPickup: 0.2, ageSeconds: 360},
		seedRider{id: "outside-radius", online: true, available: true, kmFromPickup: 8.0, ageSeconds: 30},
		seedRider{id: "no-location", online: true, available: true, noLocation: true},
		// Account rules: only an active, non-deleted rider is ever offered work.
		seedRider{id: "customer-account", online: true, available: true, kmFromPickup: 0.1, ageSeconds: 30, role: "customer"},
		seedRider{id: "suspended", online: true, available: true, kmFromPickup: 0.1, ageSeconds: 30, status: "suspended"},
		seedRider{id: "deleted", online: true, available: true, kmFromPickup: 0.1, ageSeconds: 30, deleted: true},
	)
	repo := NewDeliveryRepository(db)

	got := ids(t, repo, 10)

	want := []string{"eligible-near", "eligible-4min", "eligible-far"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v (nearest first)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (nearest first)", got, want)
		}
	}
}

func TestFindNearestRidersOnPostgresHonoursTheLimitAndDistance(t *testing.T) {
	db := nearestDB(t)
	seed(t, db,
		seedRider{id: "a", online: true, available: true, kmFromPickup: 0.5, ageSeconds: 10},
		seedRider{id: "b", online: true, available: true, kmFromPickup: 1.5, ageSeconds: 10},
		seedRider{id: "c", online: true, available: true, kmFromPickup: 2.5, ageSeconds: 10},
	)
	repo := NewDeliveryRepository(db)

	riders, err := repo.FindNearestRiders(context.Background(), pickupLat, pickupLng, 5.0, 2)
	if err != nil {
		t.Fatalf("FindNearestRiders: %v", err)
	}
	if len(riders) != 2 || nameOf(riders[0].RiderID) != "a" || nameOf(riders[1].RiderID) != "b" {
		t.Fatalf("unexpected %+v", riders)
	}
	if riders[0].DistanceKm < 0.4 || riders[0].DistanceKm > 0.6 {
		t.Fatalf("distance %.2f km, want about 0.5", riders[0].DistanceKm)
	}
}

// A rider exactly at the pickup must not produce acos(>1) = NaN.
func TestFindNearestRidersOnPostgresHandlesARiderAtThePickup(t *testing.T) {
	db := nearestDB(t)
	seed(t, db, seedRider{id: "here", online: true, available: true, kmFromPickup: 0, ageSeconds: 5})

	got := ids(t, NewDeliveryRepository(db), 5)

	if len(got) != 1 || got[0] != "here" {
		t.Fatalf("got %v", got)
	}
}

// The Redis fast path re-checks candidates with the same rules. A stale or
// busy rider that Redis still lists must be dropped.
func TestFindNearestRidersAmongOnPostgresReChecksRedisCandidates(t *testing.T) {
	db := nearestDB(t)
	busy := 7
	seed(t, db,
		seedRider{id: "ok", online: true, available: true, kmFromPickup: 0.5, ageSeconds: 10},
		seedRider{id: "busy", online: true, available: true, currentOrder: &busy, kmFromPickup: 0.3, ageSeconds: 10},
		seedRider{id: "stale", online: true, available: true, kmFromPickup: 0.3, ageSeconds: 900},
		seedRider{id: "suspended", online: true, available: true, kmFromPickup: 0.3, ageSeconds: 10, status: "suspended"},
		seedRider{id: "not-a-candidate", online: true, available: true, kmFromPickup: 0.1, ageSeconds: 10},
	)
	repo := NewDeliveryRepository(db)

	candidates := []string{riderUUID("ok"), riderUUID("busy"), riderUUID("stale"), riderUUID("suspended"), riderUUID("unknown")}
	riders, err := repo.FindNearestRidersAmong(context.Background(), pickupLat, pickupLng, 5.0, 5, candidates)
	if err != nil {
		t.Fatalf("FindNearestRidersAmong must execute on PostgreSQL: %v", err)
	}
	if len(riders) != 1 || nameOf(riders[0].RiderID) != "ok" {
		t.Fatalf("got %+v, want only the eligible candidate", riders)
	}
}

// Every other query the dispatch path runs, executed once on PostgreSQL.
func TestDispatchQueriesExecuteOnPostgres(t *testing.T) {
	db := nearestDB(t)
	seed(t, db, seedRider{id: "r1", online: true, available: true, kmFromPickup: 0.5, ageSeconds: 10})
	repo := NewDeliveryRepository(db)
	ctx := context.Background()
	r1 := riderUUID("r1")

	summary, err := repo.GetRiderEligibilitySummary(ctx, pickupLat, pickupLng, 5.0)
	if err != nil {
		t.Fatalf("GetRiderEligibilitySummary: %v", err)
	}
	if summary.OnlineRiders != 1 || summary.RidersWithinRadius != 1 {
		t.Fatalf("summary = %+v, want the one seeded rider counted online and within the radius", summary)
	}
	decision, err := repo.RiderDecisionVector(ctx, r1, pickupLat, pickupLng, 5.0)
	if err != nil {
		t.Fatalf("RiderDecisionVector: %v", err)
	}
	if decision.FirstFailure() != "" {
		t.Fatalf("r1 should pass every filter, failed %q", decision.FirstFailure())
	}
	if _, err := repo.DeclinedRiderIDs(ctx, 1); err != nil {
		t.Fatalf("DeclinedRiderIDs: %v", err)
	}
	if _, err := repo.GetPendingRequestsForRider(ctx, r1); err != nil {
		t.Fatalf("GetPendingRequestsForRider: %v", err)
	}
}

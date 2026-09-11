package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// HasActiveRestaurantOwnRiders once joined `rr.rider_user_id = u.id` — a
// varchar compared with a uuid, which Postgres rejects — so it failed on every
// call. It also read users.is_available, a flag that never expires.

func captureOwnRiderSQL(t *testing.T) string {
	t.Helper()
	var captured string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(_, actual string) error {
			captured = actual
			return nil
		})))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(".").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	if _, err := NewRiderRepository(db).HasActiveRestaurantOwnRiders(context.Background(), 27); err != nil {
		t.Fatalf("HasActiveRestaurantOwnRiders: %v", err)
	}
	return captured
}

func TestOwnRiderCheckCastsTheUserIDForTheJoin(t *testing.T) {
	sql := captureOwnRiderSQL(t)
	if !strings.Contains(sql, "u.id::text = rr.rider_user_id") {
		t.Fatalf("users must be joined with an explicit cast; varchar = uuid fails:\n%s", sql)
	}
	if strings.Contains(sql, "rr.rider_user_id = u.id\n") || strings.Contains(sql, "rr.rider_user_id = u.id ") {
		t.Fatalf("the uncast join that failed on every call is back:\n%s", sql)
	}
}

// The same definition of "live" this service's dispatch query uses.
func TestOwnRiderCheckRequiresARecentLocation(t *testing.T) {
	sql := captureOwnRiderSQL(t)
	for _, clause := range []string{
		"ra.is_online = true",
		"ra.is_available = true",
		"ra.current_order_id IS NULL",
		"rl.last_updated_at >= NOW() - INTERVAL '5 minutes'",
	} {
		if !strings.Contains(sql, clause) {
			t.Errorf("own-rider check is missing %q", clause)
		}
	}
}

func TestOwnRiderCheckNeverReadsTheStaleAvailabilityFlags(t *testing.T) {
	sql := captureOwnRiderSQL(t)
	for _, stale := range []string{"u.is_available", "u.on_trip"} {
		if strings.Contains(sql, stale) {
			t.Errorf("own-rider check reads %q, which never expires", stale)
		}
	}
}

// All three windows — this check, this service's dispatch query, and
// restaurant-service's — must agree.
func TestOwnRiderLivenessMatchesTheDispatchQuery(t *testing.T) {
	if OwnRiderLivenessInterval != "5 minutes" {
		t.Fatalf("OwnRiderLivenessInterval = %q; dispatch uses 5 minutes", OwnRiderLivenessInterval)
	}
}

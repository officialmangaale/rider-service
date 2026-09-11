package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// Each guard in redispatchCandidateSQL stops riders being offered an order
// they must not get. These tests pin them so a later edit cannot drop one.

func TestRedispatchNeverOffersAnAssignedOrRestaurantOwnedOrder(t *testing.T) {
	for _, guard := range []string{
		"d.assigned_rider_id IS NULL",
		"COALESCE(d.rider_user_id, '') = ''",
		"COALESCE(d.restaurant_owned, FALSE) = FALSE",
		"COALESCE(o.assigned_rider_user_id, '') = ''",
	} {
		if !strings.Contains(redispatchCandidateSQL, guard) {
			t.Errorf("candidate query lost the assignment guard %q", guard)
		}
	}
}

// delivery_orders still says no_rider_found for orders the restaurant has
// since cancelled or completed; the restaurant order's status is the gate.
func TestRedispatchOnlyOffersOrdersTheRestaurantStillWantsDelivered(t *testing.T) {
	want := "LOWER(o.order_status) IN ('accepted', 'confirmed', 'preparing', 'ready')"
	if !strings.Contains(redispatchCandidateSQL, want) {
		t.Fatalf("candidate query must gate on the restaurant order status:\n%s", redispatchCandidateSQL)
	}
	for _, terminal := range []string{"'cancelled'", "'delivered'", "'completed'"} {
		if strings.Contains(strings.SplitN(redispatchCandidateSQL, "NOT EXISTS", 2)[0], terminal) {
			t.Errorf("candidate query must not select terminal status %s", terminal)
		}
	}
	if !strings.Contains(redispatchCandidateSQL, "o.is_deleted = FALSE") || !strings.Contains(redispatchCandidateSQL, "d.is_deleted = FALSE") {
		t.Error("candidate query must skip soft-deleted rows")
	}
}

// Production holds months-old orders that were never closed and still read
// "confirmed"; the age ceiling is what keeps riders from being rung for them.
func TestRedispatchHasAnAgeCeiling(t *testing.T) {
	if !strings.Contains(redispatchCandidateSQL, "d.created_at >= NOW() - make_interval(secs => $1)") {
		t.Fatalf("candidate query must bound order age:\n%s", redispatchCandidateSQL)
	}
}

// A pending offer must be left to run, and an accepted one ends the search.
func TestRedispatchLeavesLiveOffersAlone(t *testing.T) {
	want := "r.status = 'accepted'\n\t\t                OR r.expires_at > NOW() - make_interval(secs => $2)"
	if !strings.Contains(redispatchCandidateSQL, want) {
		t.Fatalf("candidate query must skip orders with a live, recent or accepted offer:\n%s", redispatchCandidateSQL)
	}
}

func TestFindRedispatchCandidatesPassesAgeCooldownAndLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM delivery_orders d")).
		WithArgs(float64(7200), float64(60), 50).
		WillReturnRows(sqlmock.NewRows([]string{"delivery_order_id", "order_id"}).
			AddRow(15, 13283).
			AddRow(16, 13286))

	got, err := NewDeliveryRepository(db).FindRedispatchCandidates(context.Background(), 2*time.Hour, time.Minute, 50)
	if err != nil {
		t.Fatalf("FindRedispatchCandidates: %v", err)
	}
	if len(got) != 2 || got[0] != (RedispatchCandidate{DeliveryOrderID: 15, OrderID: 13283}) || got[1] != (RedispatchCandidate{DeliveryOrderID: 16, OrderID: 13286}) {
		t.Fatalf("unexpected candidates %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Only an explicit decline excludes a rider; an offer that merely expired
// does not.
func TestDeclinedRiderIDsReadsOnlyRejectedRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("WHERE delivery_order_id = $1 AND status = 'rejected'")).
		WithArgs(16).
		WillReturnRows(sqlmock.NewRows([]string{"rider_id"}).AddRow("rider-a"))

	got, err := NewDeliveryRepository(db).DeclinedRiderIDs(context.Background(), 16)
	if err != nil {
		t.Fatalf("DeclinedRiderIDs: %v", err)
	}
	if len(got) != 1 || !got["rider-a"] {
		t.Fatalf("unexpected declined set %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// Orders 13294 and 13312: FindNearestRiders failed with a SQL error and the
// order was marked no_rider_found, exactly as if nobody were nearby. The
// only difference was one free-text line. These tests pin the structured
// events that now tell the two apart.

func captureDispatch(t *testing.T) *[]string {
	t.Helper()
	// Emit may run on several goroutines at once (concurrent accepts), so the
	// capture is locked like the real logger.
	var lines []string
	var mu sync.Mutex
	restore := dispatchtrace.SetOutput(func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(format, args...))
	})
	t.Cleanup(restore)
	return &lines
}

func expectFunnel(mock sqlmock.Sqlmock, online, available, withLocation, fresh, within int) {
	mock.ExpectQuery(regexp.QuoteMeta("WITH rider_candidates AS")).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e"}).
			AddRow(online, available, withLocation, fresh, within))
}

const pickupLat, pickupLng = 28.4139396, 77.0422375

func TestAFailedSearchIsLoggedDifferentlyFromNoRiders(t *testing.T) {
	svc, mock := newRedispatchService(t)
	lines := captureDispatch(t)
	expectFunnel(mock, 1, 1, 1, 1, 1)

	svc.traceEligibility(context.Background(), "order_event", 13294, 17, pickupLat, pickupLng, nil,
		errors.New(`pq: column "rl.rider_id" must appear in the GROUP BY clause or be used in an aggregate function`))

	line := strings.Join(*lines, "\n")
	for _, want := range []string{
		"event=dispatch.eligibility.evaluated",
		"result=error",
		"reason_code=eligibility_query_failed",
		"would_be_eligible=1",
		"GROUP_BY",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %q", want, line)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNoRidersReportsWhichFilterRemovedThem(t *testing.T) {
	svc, mock := newRedispatchService(t)
	lines := captureDispatch(t)
	// 6 online, all available, all with a location, 1 fresh, 1 within radius.
	expectFunnel(mock, 6, 6, 6, 1, 0)

	svc.traceEligibility(context.Background(), "order_event", 13312, 18, pickupLat, pickupLng, nil, nil)

	line := (*lines)[0]
	for _, want := range []string{
		"event=dispatch.no_eligible_riders",
		"reason_code=no_eligible_riders",
		"rejected_counts=rider_location_missing:0,rider_location_stale:5,rider_not_available:0,rider_outside_radius:1",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %q", want, line)
		}
	}
}

// Normal traffic logs only aggregates. The per-rider vector needs an
// explicit, unexpired trace for that order.
func TestTheTargetRiderVectorNeedsAnActiveTrace(t *testing.T) {
	svc, mock := newRedispatchService(t)
	lines := captureDispatch(t)
	riders := []models.NearbyRider{{RiderID: "rider-1", DistanceKm: 0.2}}

	svc.traceEligibility(context.Background(), "order_event", 13294, 17, pickupLat, pickupLng, riders, nil)
	if len(*lines) != 1 || strings.Contains((*lines)[0], "target_rider") {
		t.Fatalf("no vector without a trace: %q", *lines)
	}

	svc.SetTrace(dispatchtrace.Trace{OrderID: 13294, RiderID: "rider-1", Until: time.Now().Add(time.Hour)})
	mock.ExpectQuery(regexp.QuoteMeta("FROM (SELECT $1::text AS rider_id) target")).
		WithArgs("rider-1", pickupLat, pickupLng).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e", "f", "g", "h"}).
			AddRow(true, true, true, true, true, true, int64(40), 0.2))
	*lines = nil

	svc.traceEligibility(context.Background(), "order_event", 13294, 17, pickupLat, pickupLng, riders, nil)

	vector := (*lines)[len(*lines)-1]
	for _, want := range []string{
		"event=dispatch.eligibility.target_rider",
		"eligible=true",
		"selected_by_search=true",
		"location_age_s=40",
	} {
		if !strings.Contains(vector, want) {
			t.Errorf("missing %q in %q", want, vector)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Coordinates must never reach a log line, only distances and ages.
func TestDispatchEventsNeverContainCoordinates(t *testing.T) {
	svc, mock := newRedispatchService(t)
	lines := captureDispatch(t)
	expectFunnel(mock, 1, 1, 1, 1, 1)
	svc.SetTrace(dispatchtrace.Trace{RiderID: "rider-1", Until: time.Now().Add(time.Hour)})
	mock.ExpectQuery(regexp.QuoteMeta("target")).
		WillReturnRows(sqlmock.NewRows([]string{"a", "b", "c", "d", "e", "f", "g", "h"}).
			AddRow(true, true, true, true, true, false, int64(12035), 0.0023))

	svc.traceEligibility(context.Background(), "order_event", 13294, 17, pickupLat, pickupLng, nil, errors.New("boom"))

	all := strings.Join(*lines, "\n")
	for _, secret := range []string{"28.41", "77.04", "4139396", "0422375"} {
		if strings.Contains(all, secret) {
			t.Fatalf("a coordinate fragment %q reached the log: %s", secret, all)
		}
	}
}

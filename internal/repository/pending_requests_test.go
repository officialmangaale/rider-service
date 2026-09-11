package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// GET /api/v1/riders/order-requests is the snapshot the rider app loads on
// launch, on resume, after a reconnect, and from the online foreground
// service while the app is in the background. It must return exactly the
// rider's live offers: an expired or answered one would ring for nothing.
func TestPendingRequestSnapshotReturnsOnlyLiveOffersForTheRider(t *testing.T) {
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

	now := time.Now()
	mock.ExpectQuery(".").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"request_id", "delivery_order_id", "order_id", "rider_id", "status", "distance_km", "expires_at", "created_at", "updated_at"}).
			AddRow(901, 16, 13286, "rider-1", "pending", 0.4, now.Add(20*time.Second), now, now))

	got, err := NewDeliveryRepository(db).GetPendingRequestsForRider(context.Background(), "rider-1")
	if err != nil {
		t.Fatalf("GetPendingRequestsForRider: %v", err)
	}
	if len(got) != 1 || got[0].RequestID != 901 {
		t.Fatalf("unexpected snapshot %+v", got)
	}
	for _, guard := range []string{"rider_id=$1", "status='pending'", "expires_at > NOW()"} {
		if !regexp.MustCompile(regexp.QuoteMeta(guard)).MatchString(captured) {
			t.Errorf("snapshot query lost the %q guard:\n%s", guard, captured)
		}
	}
}

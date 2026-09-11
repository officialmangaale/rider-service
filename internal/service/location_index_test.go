package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// The app uploads to POST /api/v1/location/update (LocationService). Only
// POST /api/v1/riders/location used to reach the Redis dispatch index, so it
// stayed empty. Both now index after their PostgreSQL write succeeds.

type recordingIndexer struct{ calls []string }

func (r *recordingIndexer) IndexRiderLocation(_ context.Context, riderID string, _, _ float64) {
	r.calls = append(r.calls, riderID)
}

func TestAppLocationUploadIsIndexedAfterPostgres(t *testing.T) {
	svc, mock := newLocationService(t)
	indexer := &recordingIndexer{}
	svc.SetLocationIndexer(indexer)
	expectUsersLocationUpdate(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO rider_locations")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("rider_location_history")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if _, err := svc.UpdateLocation(context.Background(), "rider-1", 28.45, 77.02, nil, nil); err != nil {
		t.Fatalf("UpdateLocation: %v", err)
	}
	if len(indexer.calls) != 1 || indexer.calls[0] != "rider-1" {
		t.Fatalf("indexer calls %v", indexer.calls)
	}
}

// PostgreSQL is the source of truth: nothing is indexed, and the upload
// fails, when the dispatch location was not stored.
func TestFailedLocationWriteIsNeitherIndexedNorReportedAsSuccess(t *testing.T) {
	svc, mock := newLocationService(t)
	indexer := &recordingIndexer{}
	svc.SetLocationIndexer(indexer)
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users SET current_lat")).
		WillReturnRows(sqlmock.NewRows([]string{"last_location_update", "is_available"}).AddRow(now, true))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO rider_locations")).
		WillReturnError(errors.New("connection reset"))

	if _, err := svc.UpdateLocation(context.Background(), "rider-1", 28.45, 77.02, nil, nil); err == nil {
		t.Fatal("a failed dispatch-location write must fail the upload")
	}
	if len(indexer.calls) != 0 {
		t.Fatalf("indexed without a stored location: %v", indexer.calls)
	}
}

// Without Redis configured, indexing is a no-op and never fails an upload.
func TestIndexRiderLocationWithoutRedisIsANoOp(t *testing.T) {
	svc, _ := newRedispatchService(t) // built with a nil cache
	svc.IndexRiderLocation(context.Background(), "rider-1", 28.45, 77.02)
}

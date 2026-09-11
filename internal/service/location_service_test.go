package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

func newLocationService(t *testing.T) (*LocationService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewLocationService(repository.NewRiderRepository(db), repository.NewLocationHistoryRepository(db)), mock
}

func expectUsersLocationUpdate(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE users SET current_lat")).
		WithArgs("rider-1", 28.45, 77.02).
		WillReturnRows(sqlmock.NewRows([]string{"last_location_update", "is_available"}).AddRow(time.Now(), true))
}

// A background update keeps the rider eligible only if it reaches
// rider_locations, the table dispatch reads.
func TestLocationUpdateWritesTheDispatchLocation(t *testing.T) {
	svc, mock := newLocationService(t)
	expectUsersLocationUpdate(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO rider_locations")).
		WithArgs("rider-1", 28.45, 77.02).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("rider_location_history")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if _, err := svc.UpdateLocation(context.Background(), "rider-1", 28.45, 77.02, nil, nil); err != nil {
		t.Fatalf("UpdateLocation: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// This error used to be discarded: the app heard "updated" while dispatch
// still saw the old fix, so the rider silently went stale.
func TestLocationUpdateFailsWhenTheDispatchLocationWriteFails(t *testing.T) {
	svc, mock := newLocationService(t)
	expectUsersLocationUpdate(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO rider_locations")).
		WillReturnError(errors.New("connection reset"))

	if _, err := svc.UpdateLocation(context.Background(), "rider-1", 28.45, 77.02, nil, nil); err == nil {
		t.Fatal("expected an error so the app retries the update")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

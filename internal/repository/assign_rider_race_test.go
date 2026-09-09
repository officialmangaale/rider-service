package repository

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// AssignRider is the single point that makes rider assignment atomic: the
// UPDATE is guarded by `assigned_rider_id IS NULL`, so when two riders accept
// the same delivery order concurrently Postgres serialises the two UPDATEs on
// the row and the loser matches zero rows. These tests pin that contract —
// dropping the guard, or ignoring RowsAffected, would let both riders be
// assigned to one order.

func newRepo(t *testing.T) (*DeliveryRepository, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return NewDeliveryRepository(db), mock, db
}

func TestAssignRiderGuardsOnUnassignedRow(t *testing.T) {
	repo, mock, db := newRepo(t)
	defer db.Close()

	mock.ExpectExec("UPDATE delivery_orders").
		WithArgs(77, "rider-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.AssignRider(context.Background(), nil, 77, "rider-a"); err != nil {
		t.Fatalf("first accept must win: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The losing rider's UPDATE matches no row, and that MUST surface as an error
// rather than a silent success.
func TestAssignRiderRejectsSecondRider(t *testing.T) {
	repo, mock, db := newRepo(t)
	defer db.Close()

	mock.ExpectExec("UPDATE delivery_orders").
		WithArgs(77, "rider-b").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.AssignRider(context.Background(), nil, 77, "rider-b")
	if err == nil {
		t.Fatal("a late accept must be rejected, not silently ignored")
	}
	if !strings.Contains(err.Error(), "already assigned") {
		t.Fatalf("error must say the order is taken, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Guards against someone "simplifying" the WHERE clause later: sqlmock only
// matches if the statement really contains the guard.
func TestAssignRiderSQLKeepsTheNullGuard(t *testing.T) {
	repo, mock, db := newRepo(t)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("assigned_rider_id IS NULL")).
		WithArgs(77, "rider-a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.AssignRider(context.Background(), nil, 77, "rider-a"); err != nil {
		t.Fatalf("AssignRider must keep the `assigned_rider_id IS NULL` guard: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

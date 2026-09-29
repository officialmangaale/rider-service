package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestListRidersComputesWalletDerivedFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	settledAt := time.Now()
	cols := []string{
		"id", "user_id", "phone", "first_name", "last_name", "display_name",
		"is_online", "is_available", "current_order_id",
		"completed", "cancelled",
		"total_earnings", "total_incentives", "total_penalties",
		"balance",
		"last_settlement_at", "is_full_and_final",
		"latitude", "longitude", "location_updated_at",
	}
	mock.ExpectQuery("FROM users u").
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			"rider-1", "U123", "9999999999", "John", "Doe", "",
			true, false, nil,
			5, 1,
			500.0, 50.0, 20.0,
			-200.0,
			settledAt, true,
			nil, nil, nil,
		))

	riders, total, err := ListRiders(context.Background(), db, ListRidersFilter{Limit: 20})
	if err != nil {
		t.Fatalf("ListRiders returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(riders) != 1 {
		t.Fatalf("expected 1 rider, got %d", len(riders))
	}

	r := riders[0]
	if r.Name != "John Doe" {
		t.Errorf("Name = %q, want %q", r.Name, "John Doe")
	}
	if !r.IsNegativeWallet {
		t.Error("expected IsNegativeWallet = true for balance -200")
	}
	if r.PlatformReceivable != 200.0 {
		t.Errorf("PlatformReceivable = %v, want 200", r.PlatformReceivable)
	}
	if r.CODPending != 200.0 {
		t.Errorf("CODPending = %v, want 200", r.CODPending)
	}
	if r.RiderPayable != 0 {
		t.Errorf("RiderPayable = %v, want 0 for a negative-balance rider", r.RiderPayable)
	}
	if r.LastSettlementStatus != "full_and_final" {
		t.Errorf("LastSettlementStatus = %q, want full_and_final", r.LastSettlementStatus)
	}
	if r.LastSettlementAt == nil {
		t.Error("expected LastSettlementAt to be set")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListRidersNeverSettledWhenNoSettlementRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	cols := []string{
		"id", "user_id", "phone", "first_name", "last_name", "display_name",
		"is_online", "is_available", "current_order_id",
		"completed", "cancelled",
		"total_earnings", "total_incentives", "total_penalties",
		"balance",
		"last_settlement_at", "is_full_and_final",
		"latitude", "longitude", "location_updated_at",
	}
	mock.ExpectQuery("FROM users u").
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			"rider-2", "U124", "8888888888", "Jane", "Roe", "",
			false, true, nil,
			0, 0,
			0.0, 0.0, 0.0,
			150.0,
			nil, nil,
			nil, nil, nil,
		))

	riders, _, err := ListRiders(context.Background(), db, ListRidersFilter{Limit: 20})
	if err != nil {
		t.Fatalf("ListRiders returned error: %v", err)
	}
	r := riders[0]
	if r.IsNegativeWallet {
		t.Error("expected IsNegativeWallet = false for positive balance")
	}
	if r.RiderPayable != 150.0 {
		t.Errorf("RiderPayable = %v, want 150", r.RiderPayable)
	}
	if r.LastSettlementStatus != "never_settled" {
		t.Errorf("LastSettlementStatus = %q, want never_settled", r.LastSettlementStatus)
	}
}

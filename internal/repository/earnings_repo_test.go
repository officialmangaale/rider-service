package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetSummaryIncludesRealWalletFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM rider_earnings").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"today_earnings", "week_earnings", "month_earnings", "total_earnings",
			"total_orders", "delivery_earnings", "tip_earnings", "incentive_earnings",
			"bonus_earnings", "penalty_amount",
		}).AddRow(30.0, 90.0, 300.0, 1000.0, 20, 800.0, 100.0, 50.0, 20.0, 30.0))

	mock.ExpectQuery("SELECT balance FROM rider_wallet_balances").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(450.0))

	mock.ExpectQuery("FROM rider_settlements").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(200.0))

	repo := NewEarningsRepository(db)
	summary, err := repo.GetSummary(context.Background(), "rider-1")
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if summary.WalletBalance != 450.0 {
		t.Errorf("WalletBalance = %v, want 450", summary.WalletBalance)
	}
	if summary.PendingPayout != 450.0 {
		t.Errorf("PendingPayout = %v, want 450 (positive balance)", summary.PendingPayout)
	}
	if summary.SettledPayout != 200.0 {
		t.Errorf("SettledPayout = %v, want 200", summary.SettledPayout)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetSummaryPendingPayoutIsZeroWhenRiderOwesPlatform(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM rider_earnings").
		WithArgs("rider-2").
		WillReturnRows(sqlmock.NewRows([]string{
			"today_earnings", "week_earnings", "month_earnings", "total_earnings",
			"total_orders", "delivery_earnings", "tip_earnings", "incentive_earnings",
			"bonus_earnings", "penalty_amount",
		}).AddRow(0.0, 0.0, 0.0, 0.0, 0, 0.0, 0.0, 0.0, 0.0, 0.0))

	mock.ExpectQuery("SELECT balance FROM rider_wallet_balances").
		WithArgs("rider-2").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(-500.0))

	mock.ExpectQuery("FROM rider_settlements").
		WithArgs("rider-2").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(0.0))

	repo := NewEarningsRepository(db)
	summary, err := repo.GetSummary(context.Background(), "rider-2")
	if err != nil {
		t.Fatalf("GetSummary returned error: %v", err)
	}
	if summary.PendingPayout != 0 {
		t.Errorf("PendingPayout = %v, want 0 for a negative wallet balance", summary.PendingPayout)
	}
	if summary.WalletBalance != -500.0 {
		t.Errorf("WalletBalance = %v, want -500", summary.WalletBalance)
	}
}

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostWalletTransactionComputesBalanceAfter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO rider_wallet_balances").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(100.0))
	mock.ExpectQuery("INSERT INTO rider_wallet_transactions").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), time.Now()))
	mock.ExpectExec("UPDATE rider_wallet_balances").
		WithArgs("rider-1", 130.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	txn, err := PostWalletTransaction(context.Background(), tx, "rider-1", "delivery_earning", 30.0, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("PostWalletTransaction returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if txn.BalanceBefore != 100.0 || txn.BalanceAfter != 130.0 {
		t.Fatalf("expected balance 100 -> 130, got %v -> %v", txn.BalanceBefore, txn.BalanceAfter)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostWalletTransactionAppliesNegativeAmountForCashCollected(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO rider_wallet_balances").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(0.0))
	mock.ExpectQuery("INSERT INTO rider_wallet_transactions").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(2), time.Now()))
	mock.ExpectExec("UPDATE rider_wallet_balances").
		WithArgs("rider-1", -2000.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	// Mirrors the module's own example: collecting ₹2,000 COD cash moves the
	// wallet to -₹2,000.
	txn, err := PostWalletTransaction(context.Background(), tx, "rider-1", "cash_collected", -2000.0, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("PostWalletTransaction returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if txn.BalanceAfter != -2000.0 {
		t.Fatalf("expected balance -2000, got %v", txn.BalanceAfter)
	}
}

func TestPostWalletTransactionRejectsUnknownType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectBegin()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	_, err = PostWalletTransaction(context.Background(), tx, "rider-1", "bogus_type", 10, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for unsupported transaction type")
	}
}

func TestPostWalletTransactionRequiresTx(t *testing.T) {
	_, err := PostWalletTransaction(context.Background(), nil, "rider-1", "penalty", -10, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when tx is nil")
	}
}

func TestCreateSettlementComputesNetEffectAndClosingBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// previous-balance read (outside PostWalletTransaction)
	mock.ExpectQuery("INSERT INTO rider_wallet_balances").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(-2000.0))
	// PostWalletTransaction's own lock-and-read (same statement pattern; sqlmock
	// matches by regex so this reuses the same expectation form)
	mock.ExpectQuery("INSERT INTO rider_wallet_balances").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(-2000.0))
	mock.ExpectQuery("INSERT INTO rider_wallet_transactions").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(3), time.Now()))
	mock.ExpectExec("UPDATE rider_wallet_balances").
		WithArgs("rider-1", 0.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO rider_settlements").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(9), time.Now()))
	mock.ExpectCommit()

	settlement, err := CreateSettlement(context.Background(), db, SettlementInput{
		RiderID:        "rider-1",
		AmountReceived: 2000.0, // rider hands over the collected COD cash
		AdminID:        "admin-1",
		IsFullAndFinal: true,
	})
	if err != nil {
		t.Fatalf("CreateSettlement returned error: %v", err)
	}
	if settlement.PreviousBalance != -2000.0 {
		t.Errorf("PreviousBalance = %v, want -2000", settlement.PreviousBalance)
	}
	if settlement.SettlementAmount != 2000.0 {
		t.Errorf("SettlementAmount (net effect) = %v, want 2000", settlement.SettlementAmount)
	}
	if settlement.ClosingBalance != 0.0 {
		t.Errorf("ClosingBalance = %v, want 0", settlement.ClosingBalance)
	}
	if settlement.WalletTransactionID == nil || *settlement.WalletTransactionID != 3 {
		t.Errorf("expected settlement linked to wallet transaction 3, got %v", settlement.WalletTransactionID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostStandaloneWalletTransactionOpensItsOwnTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO rider_wallet_balances").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(100.0))
	mock.ExpectQuery("INSERT INTO rider_wallet_transactions").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(5), time.Now()))
	mock.ExpectExec("UPDATE rider_wallet_balances").
		WithArgs("rider-1", 250.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	adminID := "admin-1"
	txn, err := PostStandaloneWalletTransaction(context.Background(), db, "rider-1", "incentive", 150.0, nil, nil, &adminID)
	if err != nil {
		t.Fatalf("PostStandaloneWalletTransaction returned error: %v", err)
	}
	if txn.BalanceAfter != 250.0 {
		t.Fatalf("expected balance 100 -> 250, got %v", txn.BalanceAfter)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCreateSettlementRequiresRiderAndAdminID(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	if _, err := CreateSettlement(context.Background(), db, SettlementInput{AdminID: "admin-1"}); err == nil {
		t.Fatal("expected error when rider id is missing")
	}
	if _, err := CreateSettlement(context.Background(), db, SettlementInput{RiderID: "rider-1"}); err == nil {
		t.Fatal("expected error when admin id is missing")
	}
}

func TestListAllWalletTransactionsAppliesRiderAndDateFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	now := time.Now()

	mock.ExpectQuery("SELECT id, rider_id, transaction_type, amount, balance_before, balance_after").
		WithArgs("rider-1", from, to, 5000).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rider_id", "transaction_type", "amount", "balance_before", "balance_after",
			"order_id", "reference_number", "payment_mode", "notes", "created_by_admin_id", "created_at",
		}).AddRow(int64(1), "rider-1", "delivery_earning", 30.0, 0.0, 30.0, nil, nil, nil, nil, nil, now))

	txns, err := ListAllWalletTransactions(context.Background(), db, AllWalletTransactionsFilter{
		RiderID: "rider-1", DateFrom: &from, DateTo: &to,
	})
	if err != nil {
		t.Fatalf("ListAllWalletTransactions returned error: %v", err)
	}
	if len(txns) != 1 || txns[0].RiderID != "rider-1" || txns[0].Amount != 30.0 {
		t.Fatalf("unexpected transactions: %+v", txns)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListAllSettlementsAppliesRiderFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery("SELECT id, rider_id, previous_balance, settlement_amount, amount_received, amount_paid").
		WithArgs("rider-1", 5000).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rider_id", "previous_balance", "settlement_amount", "amount_received", "amount_paid",
			"adjustment_amount", "closing_balance", "payment_mode", "reference_number", "is_full_and_final",
			"notes", "admin_id", "wallet_transaction_id", "created_at",
		}).AddRow(int64(9), "rider-1", -2000.0, 2000.0, 2000.0, 0.0, 0.0, 0.0, nil, nil, true, nil, "admin-1", nil, now))

	settlements, err := ListAllSettlements(context.Background(), db, AllSettlementsFilter{RiderID: "rider-1"})
	if err != nil {
		t.Fatalf("ListAllSettlements returned error: %v", err)
	}
	if len(settlements) != 1 || settlements[0].RiderID != "rider-1" || settlements[0].ClosingBalance != 0.0 {
		t.Fatalf("unexpected settlements: %+v", settlements)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

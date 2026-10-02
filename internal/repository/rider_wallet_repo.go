package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// PostWalletTransaction is the single choke point for every rider wallet
// balance change (platform upgrade Module 11) — the wallet balance is never
// written directly by anything else. Locks (and upserts if missing) the
// rider's rider_wallet_balances row FOR UPDATE inside tx, so two concurrent
// postings for the same rider serialize correctly, mirroring the pattern
// restaurant-service already uses for restaurants.wallet_amount +
// billing_ledger (see round_off.go there).
//
// tx is required — callers already inside a delivery transition pass their
// existing tx; standalone callers (e.g. settlement recording) open their own.
func PostWalletTransaction(ctx context.Context, tx *sql.Tx, riderID, transactionType string, amount float64,
	orderID *int, referenceNumber, paymentMode, notes, createdByAdminID *string) (*models.RiderWalletTransaction, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	if riderID == "" {
		return nil, errors.New("rider id required")
	}
	if !validWalletTxnType(transactionType) {
		return nil, fmt.Errorf("unsupported wallet transaction type %q", transactionType)
	}

	// INSERT ... ON CONFLICT DO UPDATE ... RETURNING acquires and holds a
	// row-level lock on the (existing-or-just-created) row for the rest of
	// this transaction, exactly like SELECT ... FOR UPDATE would — so two
	// concurrent postings for the same rider serialize correctly without a
	// second, redundant lock statement.
	var before float64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO rider_wallet_balances (rider_id, balance)
		VALUES ($1, 0)
		ON CONFLICT (rider_id) DO UPDATE SET rider_id = EXCLUDED.rider_id
		RETURNING balance
	`, riderID).Scan(&before)
	if err != nil {
		return nil, fmt.Errorf("lock rider wallet balance: %w", err)
	}

	after := before + amount

	var txnID int64
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO rider_wallet_transactions (
			rider_id, transaction_type, amount, balance_before, balance_after,
			order_id, reference_number, payment_mode, notes, created_by_admin_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		RETURNING id, created_at
	`, riderID, transactionType, amount, before, after, orderID, referenceNumber, paymentMode, notes, createdByAdminID,
	).Scan(&txnID, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert rider wallet transaction: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE rider_wallet_balances SET balance = $2, updated_at = NOW() WHERE rider_id = $1
	`, riderID, after); err != nil {
		return nil, fmt.Errorf("update rider wallet balance: %w", err)
	}

	return &models.RiderWalletTransaction{
		ID: txnID, RiderID: riderID, TransactionType: transactionType, Amount: amount,
		BalanceBefore: before, BalanceAfter: after, OrderID: orderID,
		ReferenceNumber: referenceNumber, PaymentMode: paymentMode, Notes: notes,
		CreatedByAdminID: createdByAdminID, CreatedAt: createdAt,
	}, nil
}

// PostStandaloneWalletTransaction records one admin-initiated wallet entry
// outside of a settlement (e.g. an incentive, penalty, or manual
// correction) — opens its own transaction since, unlike a delivery-time
// posting, this is not part of a larger caller-owned transaction.
func PostStandaloneWalletTransaction(ctx context.Context, db *sql.DB, riderID, transactionType string, amount float64,
	referenceNumber, notes, createdByAdminID *string) (*models.RiderWalletTransaction, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	txn, err := PostWalletTransaction(ctx, tx, riderID, transactionType, amount, nil, referenceNumber, nil, notes, createdByAdminID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return txn, nil
}

func validWalletTxnType(t string) bool {
	switch t {
	case models.WalletTxnDeliveryEarning, models.WalletTxnIncentive, models.WalletTxnCashCollected,
		models.WalletTxnCODLiability, models.WalletTxnRiderPaymentToCompany, models.WalletTxnCompanyPaymentToRider,
		models.WalletTxnPenalty, models.WalletTxnManualAdjustment, models.WalletTxnSettlement, models.WalletTxnRefundReversal:
		return true
	default:
		return false
	}
}

// GetWalletBalance returns a rider's current balance (0 if they have never
// had a transaction — no row exists yet, which is not an error).
func GetWalletBalance(ctx context.Context, db *sql.DB, riderID string) (float64, error) {
	var balance float64
	err := db.QueryRowContext(ctx, `SELECT balance FROM rider_wallet_balances WHERE rider_id = $1`, riderID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return balance, err
}

// ListWalletTransactions returns one rider's ledger, most recent first.
func ListWalletTransactions(ctx context.Context, db *sql.DB, riderID string, limit, offset int) ([]models.RiderWalletTransaction, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rider_wallet_transactions WHERE rider_id = $1`, riderID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, rider_id, transaction_type, amount, balance_before, balance_after,
			order_id, reference_number, payment_mode, notes, created_by_admin_id, created_at
		FROM rider_wallet_transactions
		WHERE rider_id = $1
		ORDER BY id DESC
		LIMIT $2 OFFSET $3
	`, riderID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []models.RiderWalletTransaction{}
	for rows.Next() {
		var t models.RiderWalletTransaction
		if err := rows.Scan(&t.ID, &t.RiderID, &t.TransactionType, &t.Amount, &t.BalanceBefore, &t.BalanceAfter,
			&t.OrderID, &t.ReferenceNumber, &t.PaymentMode, &t.Notes, &t.CreatedByAdminID, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// AllWalletTransactionsFilter narrows the cross-rider wallet-transactions
// report (platform upgrade Module 24: Reports and Exports). Zero values mean
// "no constraint on this field".
type AllWalletTransactionsFilter struct {
	RiderID  string
	DateFrom *time.Time
	DateTo   *time.Time
	Limit    int
}

// ListAllWalletTransactions returns the wallet ledger across every rider, for
// the admin report export — unlike ListWalletTransactions, which is scoped to
// one rider. Same table, same columns, just cross-rider and filterable by
// date range instead of paginated for a single-rider view.
func ListAllWalletTransactions(ctx context.Context, db *sql.DB, f AllWalletTransactionsFilter) ([]models.RiderWalletTransaction, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(expr string, value interface{}) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	if f.RiderID != "" {
		add("rider_id = $%d", f.RiderID)
	}
	if f.DateFrom != nil {
		add("created_at >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("created_at <= $%d", *f.DateTo)
	}
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, rider_id, transaction_type, amount, balance_before, balance_after,
			order_id, reference_number, payment_mode, notes, created_by_admin_id, created_at
		FROM rider_wallet_transactions
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, strings.Join(where, " AND "), len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RiderWalletTransaction{}
	for rows.Next() {
		var t models.RiderWalletTransaction
		if err := rows.Scan(&t.ID, &t.RiderID, &t.TransactionType, &t.Amount, &t.BalanceBefore, &t.BalanceAfter,
			&t.OrderID, &t.ReferenceNumber, &t.PaymentMode, &t.Notes, &t.CreatedByAdminID, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AllSettlementsFilter narrows the cross-rider settlements report (platform
// upgrade Module 24: Reports and Exports).
type AllSettlementsFilter struct {
	RiderID  string
	DateFrom *time.Time
	DateTo   *time.Time
	Limit    int
}

// ListAllSettlements returns settlement history across every rider, for the
// admin report export — unlike ListSettlements, which is scoped to one rider.
func ListAllSettlements(ctx context.Context, db *sql.DB, f AllSettlementsFilter) ([]models.RiderSettlement, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(expr string, value interface{}) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	if f.RiderID != "" {
		add("rider_id = $%d", f.RiderID)
	}
	if f.DateFrom != nil {
		add("created_at >= $%d", *f.DateFrom)
	}
	if f.DateTo != nil {
		add("created_at <= $%d", *f.DateTo)
	}
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, rider_id, previous_balance, settlement_amount, amount_received, amount_paid, adjustment_amount,
			closing_balance, payment_mode, reference_number, is_full_and_final, notes, admin_id, wallet_transaction_id, created_at
		FROM rider_settlements
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, strings.Join(where, " AND "), len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.RiderSettlement{}
	for rows.Next() {
		var s models.RiderSettlement
		if err := rows.Scan(&s.ID, &s.RiderID, &s.PreviousBalance, &s.SettlementAmount, &s.AmountReceived, &s.AmountPaid,
			&s.AdjustmentAmount, &s.ClosingBalance, &s.PaymentMode, &s.ReferenceNumber, &s.IsFullAndFinal, &s.Notes,
			&s.AdminID, &s.WalletTransactionID, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SettlementInput is the admin-facing settlement recording payload.
type SettlementInput struct {
	RiderID          string
	AmountReceived   float64 // rider -> company
	AmountPaid       float64 // company -> rider
	AdjustmentAmount float64
	PaymentMode      *string
	ReferenceNumber  *string
	IsFullAndFinal   bool
	Notes            *string
	AdminID          string
}

// CreateSettlement records one settlement: posts its net effect
// (amount_received - amount_paid + adjustment_amount) as a single
// 'settlement' wallet transaction, then stores the richer settlement record
// linked to it. Runs in its own transaction (unlike PostWalletTransaction,
// which expects a caller-supplied one) since a settlement is a standalone
// admin action, not part of a delivery transition.
func CreateSettlement(ctx context.Context, db *sql.DB, in SettlementInput) (*models.RiderSettlement, error) {
	if in.RiderID == "" {
		return nil, errors.New("rider id required")
	}
	if in.AdminID == "" {
		return nil, errors.New("admin id required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var previousBalance float64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO rider_wallet_balances (rider_id, balance) VALUES ($1, 0)
		ON CONFLICT (rider_id) DO UPDATE SET rider_id = EXCLUDED.rider_id
		RETURNING balance
	`, in.RiderID).Scan(&previousBalance); err != nil {
		return nil, fmt.Errorf("read previous balance: %w", err)
	}

	netEffect := in.AmountReceived - in.AmountPaid + in.AdjustmentAmount
	settlementNote := in.Notes
	txn, err := PostWalletTransaction(ctx, tx, in.RiderID, models.WalletTxnSettlement, netEffect,
		nil, in.ReferenceNumber, in.PaymentMode, settlementNote, &in.AdminID)
	if err != nil {
		return nil, err
	}

	settlement := &models.RiderSettlement{
		RiderID: in.RiderID, PreviousBalance: previousBalance, SettlementAmount: netEffect,
		AmountReceived: in.AmountReceived, AmountPaid: in.AmountPaid, AdjustmentAmount: in.AdjustmentAmount,
		ClosingBalance: txn.BalanceAfter, PaymentMode: in.PaymentMode, ReferenceNumber: in.ReferenceNumber,
		IsFullAndFinal: in.IsFullAndFinal, Notes: in.Notes, AdminID: in.AdminID, WalletTransactionID: &txn.ID,
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO rider_settlements (
			rider_id, previous_balance, settlement_amount, amount_received, amount_paid, adjustment_amount,
			closing_balance, payment_mode, reference_number, is_full_and_final, notes, admin_id, wallet_transaction_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`, settlement.RiderID, settlement.PreviousBalance, settlement.SettlementAmount, settlement.AmountReceived,
		settlement.AmountPaid, settlement.AdjustmentAmount, settlement.ClosingBalance, settlement.PaymentMode,
		settlement.ReferenceNumber, settlement.IsFullAndFinal, settlement.Notes, settlement.AdminID, settlement.WalletTransactionID,
	).Scan(&settlement.ID, &settlement.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert rider settlement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return settlement, nil
}

// ListSettlements returns one rider's settlement history, most recent first.
func ListSettlements(ctx context.Context, db *sql.DB, riderID string, limit, offset int) ([]models.RiderSettlement, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rider_settlements WHERE rider_id = $1`, riderID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, rider_id, previous_balance, settlement_amount, amount_received, amount_paid, adjustment_amount,
			closing_balance, payment_mode, reference_number, is_full_and_final, notes, admin_id, wallet_transaction_id, created_at
		FROM rider_settlements
		WHERE rider_id = $1
		ORDER BY id DESC
		LIMIT $2 OFFSET $3
	`, riderID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []models.RiderSettlement{}
	for rows.Next() {
		var s models.RiderSettlement
		if err := rows.Scan(&s.ID, &s.RiderID, &s.PreviousBalance, &s.SettlementAmount, &s.AmountReceived, &s.AmountPaid,
			&s.AdjustmentAmount, &s.ClosingBalance, &s.PaymentMode, &s.ReferenceNumber, &s.IsFullAndFinal, &s.Notes,
			&s.AdminID, &s.WalletTransactionID, &s.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

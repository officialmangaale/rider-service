-- Rider Service: idempotency guard for order-tied wallet transactions
-- (platform upgrade Module 23: Backend Validation and Security).
--
-- ADDITIVE ONLY. Adds one partial unique index to the existing
-- rider_wallet_transactions table (migration 010). No column is added,
-- dropped or retyped, and no existing row is touched.
--
-- Why
-- ---
-- rider_wallet_transactions is the append-only net-payable ledger posted by
-- PostWalletTransaction (internal/repository/rider_wallet_repo.go). The two
-- transaction types tied 1:1 to a single delivery-completion event
-- ('delivery_earning' and 'cash_collected', posted from AdvanceDelivery in
-- internal/repository/delivery_transition.go) are already guarded at the
-- application layer by a `SELECT ... FOR UPDATE` lock on the delivery order
-- row plus a `current == next` no-op check, so an ordinary retried status
-- transition cannot re-post them today. This index is defence in depth at
-- the ledger itself, matching the guard billing_ledger and
-- restaurant_payout_transactions already have in restaurant-service
-- (idx_billing_ledger_order_deduction, idx_restaurant_payout_txn_order_settlement)
-- and the one rider_earnings already has for referral bonuses (migration 009):
-- a bug or a second call site added later cannot double-credit or
-- double-debit a rider for the same order, even if the application-level
-- guard is ever bypassed.
--
-- The index is partial: it constrains only the two order-tied types, so
-- admin-initiated types (incentive, penalty, manual_adjustment, settlement,
-- refund_reversal, ...) — which can legitimately repeat for the same order,
-- or have no order at all — are unaffected.
--
-- Rollback: see 011_rider_wallet_transaction_idempotency.down.sql.

CREATE UNIQUE INDEX IF NOT EXISTS idx_rider_wallet_txn_order_type
    ON rider_wallet_transactions (order_id, transaction_type)
    WHERE order_id IS NOT NULL
      AND transaction_type IN ('delivery_earning', 'cash_collected');

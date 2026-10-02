-- Rollback for 011_rider_wallet_transaction_idempotency.up.sql.
--
-- Safe to run: the index is additive and nothing reads its existence;
-- dropping it only removes the defence-in-depth guard, not any data.

DROP INDEX IF EXISTS idx_rider_wallet_txn_order_type;

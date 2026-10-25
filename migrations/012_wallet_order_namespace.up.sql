BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE rider_wallet_transactions ADD COLUMN order_type TEXT NOT NULL DEFAULT 'food'
    CHECK (order_type IN ('food','grocery'));
DROP INDEX IF EXISTS idx_rider_wallet_txn_order_type;
CREATE UNIQUE INDEX idx_rider_wallet_txn_order_type
    ON rider_wallet_transactions(order_type,order_id,transaction_type)
    WHERE order_id IS NOT NULL AND transaction_type IN ('delivery_earning','cash_collected');
ALTER TABLE delivery_status_history ADD COLUMN order_type TEXT NOT NULL DEFAULT 'food'
    CHECK(order_type IN ('food','grocery'));
-- Migration 005's food-only foreign key rejects every grocery completion
-- without a coincidentally matching food ID. The delivery transition now
-- validates the canonical source and writes its explicit namespace.
DO $$ DECLARE fk RECORD; BEGIN
    FOR fk IN SELECT conname FROM pg_constraint WHERE conrelid='delivery_status_history'::regclass
        AND confrelid='orders'::regclass AND contype='f' LOOP
        EXECUTE format('ALTER TABLE delivery_status_history DROP CONSTRAINT %I',fk.conname);
    END LOOP;
END $$;
CREATE INDEX delivery_status_history_source ON delivery_status_history(order_type,order_id,created_at);
COMMIT;

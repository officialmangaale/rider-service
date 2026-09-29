-- Migration 010: Rider Wallet and Settlement (platform upgrade Module 10/11)
--
-- rider_earnings (migration 003) stays exactly as-is: it is the gross
-- earnings ledger ("how much has this rider earned"). This migration adds a
-- separate NET PAYABLE ledger — what the platform currently owes the rider
-- minus what the rider currently owes the platform (uncollected/unremitted
-- COD cash) — which rider_earnings alone cannot express since it never
-- records a rider's cash liability.
--
-- Sign convention: positive balance = platform owes the rider; negative
-- balance = the rider owes the platform. Matches the module's own example
-- (collecting ₹2,000 COD cash moves the wallet to -₹2,000).
--
-- rider_wallet_balances is a running-total summary row per rider, updated
-- ONLY by rider_wallet_transactions inserts (never written directly by
-- anything else) — the same "summary row + FOR UPDATE lock + append-only
-- ledger" pattern restaurant-service already uses for restaurants.wallet_amount
-- + billing_ledger (see restaurant-service/services/round_off.go).
--
-- Additive only. Does not touch users.earnings or rider_earnings.

CREATE TABLE IF NOT EXISTS rider_wallet_balances (
    rider_id   UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance    NUMERIC(12,2) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rider_wallet_transactions (
    id                  BIGSERIAL PRIMARY KEY,
    rider_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    transaction_type    VARCHAR(40) NOT NULL CHECK (transaction_type IN (
        'delivery_earning', 'incentive', 'cash_collected', 'cod_liability',
        'rider_payment_to_company', 'company_payment_to_rider', 'penalty',
        'manual_adjustment', 'settlement', 'refund_reversal'
    )),
    amount              NUMERIC(12,2) NOT NULL, -- signed; see sign convention above
    balance_before      NUMERIC(12,2) NOT NULL,
    balance_after       NUMERIC(12,2) NOT NULL,
    order_id            INTEGER,
    reference_number    VARCHAR(120),
    payment_mode        VARCHAR(40),
    notes               TEXT,
    created_by_admin_id UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rider_wallet_transactions_rider
    ON rider_wallet_transactions(rider_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rider_wallet_transactions_order
    ON rider_wallet_transactions(order_id) WHERE order_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS rider_settlements (
    id                    BIGSERIAL PRIMARY KEY,
    rider_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    previous_balance      NUMERIC(12,2) NOT NULL,
    settlement_amount     NUMERIC(12,2) NOT NULL,
    amount_received       NUMERIC(12,2) NOT NULL DEFAULT 0, -- rider -> company (e.g. remitted COD cash)
    amount_paid           NUMERIC(12,2) NOT NULL DEFAULT 0, -- company -> rider (e.g. payout)
    adjustment_amount     NUMERIC(12,2) NOT NULL DEFAULT 0,
    closing_balance       NUMERIC(12,2) NOT NULL,
    payment_mode          VARCHAR(40),
    reference_number      VARCHAR(120),
    is_full_and_final     BOOLEAN NOT NULL DEFAULT false,
    notes                 TEXT,
    admin_id              UUID NOT NULL,
    wallet_transaction_id BIGINT REFERENCES rider_wallet_transactions(id),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rider_settlements_rider
    ON rider_settlements(rider_id, created_at DESC);

COMMENT ON TABLE rider_wallet_transactions IS
    'Immutable per-rider net-payable ledger (platform upgrade Module 11). Never update or delete a row; corrections are new rows (e.g. refund_reversal).';
COMMENT ON TABLE rider_settlements IS
    'One row per admin-recorded settlement event; always paired with a settlement-type row in rider_wallet_transactions via wallet_transaction_id.';

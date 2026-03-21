-- Rider Service: rider_earnings table
-- Per-delivery earnings ledger. users.earnings stores aggregate only.

CREATE TABLE IF NOT EXISTS rider_earnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rider_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id INTEGER REFERENCES orders(order_id),
    type VARCHAR(30) NOT NULL
        CHECK (type IN ('delivery_fee','tip','incentive','bonus','penalty')),
    amount DECIMAL(10,2) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_earnings_rider_date
    ON rider_earnings (rider_id, created_at DESC);

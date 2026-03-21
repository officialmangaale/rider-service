-- Rider Service: delivery_assignments table
-- Tracks which rider was offered which order and their response.
-- orders.delivery_partner_id only stores who accepted — this tracks all offers.

CREATE TABLE IF NOT EXISTS delivery_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id INTEGER NOT NULL REFERENCES orders(order_id) ON DELETE CASCADE,
    rider_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','accepted','rejected','expired')),
    offered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    responded_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    reject_reason TEXT,
    UNIQUE (order_id, rider_id)
);

CREATE INDEX IF NOT EXISTS idx_assignments_rider_pending
    ON delivery_assignments (rider_id, status) WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_assignments_order
    ON delivery_assignments (order_id);

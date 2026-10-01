-- Rider support tickets: a rider reports a delivery, payout, account or safety
-- problem from the app; operations read and resolve them in the admin panel.
--
-- ADDITIVE ONLY: one new table, nothing existing is touched.
-- Apply manually: psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/012_rider_support_tickets.up.sql

CREATE TABLE IF NOT EXISTS rider_support_tickets (
    id           BIGSERIAL PRIMARY KEY,
    rider_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category     TEXT        NOT NULL DEFAULT 'other'
                             CHECK (category IN ('delivery', 'payout', 'account', 'safety', 'other')),
    subject      TEXT        NOT NULL,
    description  TEXT        NOT NULL,
    order_id     BIGINT,
    status       TEXT        NOT NULL DEFAULT 'open'
                             CHECK (status IN ('open', 'in_progress', 'resolved', 'closed')),
    admin_note   TEXT,
    handled_by   UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rider_support_tickets_rider
    ON rider_support_tickets (rider_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rider_support_tickets_status
    ON rider_support_tickets (status, created_at DESC);

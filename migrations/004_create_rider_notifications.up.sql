-- Rider Service: rider_notifications table
-- Rider-specific notification inbox.

CREATE TABLE IF NOT EXISTS rider_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rider_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    body TEXT,
    type VARCHAR(30),
    data JSONB,
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_rider_unread
    ON rider_notifications (rider_id, created_at DESC) WHERE is_read = false;

-- Rider Service: rider_location_history table
-- High-frequency GPS tracking log. users.current_lat/lng stores only latest.

CREATE TABLE IF NOT EXISTS rider_location_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rider_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    latitude DECIMAL(10,7) NOT NULL,
    longitude DECIMAL(10,7) NOT NULL,
    heading DECIMAL(5,2),
    speed DECIMAL(5,2),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_location_history_rider_time
    ON rider_location_history (rider_id, recorded_at DESC);

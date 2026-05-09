-- =====================================================
-- Migration 006: Delivery Flow Tables
-- rider-service owned tables for SQS-driven delivery
-- =====================================================

-- Rider current location (upsert table, one row per rider)
CREATE TABLE IF NOT EXISTS rider_locations (
    rider_id VARCHAR(255) PRIMARY KEY,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Rider availability status (upsert table, one row per rider)
CREATE TABLE IF NOT EXISTS rider_availability (
    rider_id VARCHAR(255) PRIMARY KEY,
    is_online BOOLEAN NOT NULL DEFAULT false,
    is_available BOOLEAN NOT NULL DEFAULT false,
    current_order_id INTEGER,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Delivery orders created from ORDER_PLACED SQS events (rider-service owned)
CREATE TABLE IF NOT EXISTS delivery_orders (
    delivery_order_id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL UNIQUE,
    restaurant_id INTEGER NOT NULL,
    customer_id INTEGER NOT NULL,
    pickup_latitude DOUBLE PRECISION NOT NULL,
    pickup_longitude DOUBLE PRECISION NOT NULL,
    pickup_address TEXT NOT NULL DEFAULT '',
    drop_latitude DOUBLE PRECISION NOT NULL,
    drop_longitude DOUBLE PRECISION NOT NULL,
    drop_address TEXT NOT NULL DEFAULT '',
    amount DOUBLE PRECISION NOT NULL DEFAULT 0,
    payment_mode VARCHAR(50) NOT NULL DEFAULT 'cod',
    delivery_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    assigned_rider_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    assigned_at TIMESTAMPTZ,
    picked_up_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ
);

-- Delivery order requests sent to nearby riders
CREATE TABLE IF NOT EXISTS delivery_order_requests (
    request_id SERIAL PRIMARY KEY,
    delivery_order_id INTEGER NOT NULL REFERENCES delivery_orders(delivery_order_id),
    order_id INTEGER NOT NULL,
    rider_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    distance_km DOUBLE PRECISION NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(delivery_order_id, rider_id)
);

-- Processed events for idempotency
CREATE TABLE IF NOT EXISTS processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    order_id INTEGER,
    event_type VARCHAR(100) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =====================================================
-- Indexes
-- =====================================================
CREATE INDEX IF NOT EXISTS idx_delivery_orders_order_id ON delivery_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_status ON delivery_orders(delivery_status);
CREATE INDEX IF NOT EXISTS idx_delivery_orders_assigned_rider ON delivery_orders(assigned_rider_id);

CREATE INDEX IF NOT EXISTS idx_delivery_order_requests_rider_id ON delivery_order_requests(rider_id);
CREATE INDEX IF NOT EXISTS idx_delivery_order_requests_status ON delivery_order_requests(status);
CREATE INDEX IF NOT EXISTS idx_delivery_order_requests_delivery_order ON delivery_order_requests(delivery_order_id);
CREATE INDEX IF NOT EXISTS idx_delivery_order_requests_expires ON delivery_order_requests(expires_at);

CREATE INDEX IF NOT EXISTS idx_rider_locations_updated ON rider_locations(last_updated_at);
CREATE INDEX IF NOT EXISTS idx_rider_availability_online ON rider_availability(is_online, is_available);

CREATE INDEX IF NOT EXISTS idx_processed_events_order_id ON processed_events(order_id);

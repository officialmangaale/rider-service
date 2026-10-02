-- TEST COPY of restaurant-service/migrations/097_online_delivery_dispatch.sql.
-- Vendored so rider-service CI does not need the sibling repository. When 097
-- changes, refresh this copy (TestVendoredMigration097... guards drift locally).
-- Shared database policy. Apply before deploying either service. No historical
-- orders, assignments or riders are rewritten. Rollback is the enabled switch.
CREATE TABLE IF NOT EXISTS online_delivery_dispatch_config (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT false,
    restaurant_ids bigint[] NOT NULL DEFAULT '{}',
    location_max_age_seconds integer NOT NULL DEFAULT 300 CHECK (location_max_age_seconds > 0),
    search_max_age_seconds integer NOT NULL DEFAULT 7200 CHECK (search_max_age_seconds > 0)
);
INSERT INTO online_delivery_dispatch_config(singleton) VALUES (true) ON CONFLICT DO NOTHING;

-- customer_web is the server-written source of the Mangaale customer checkout.
-- Unknown/missing sources require an audited backfill, never address inference.
CREATE OR REPLACE FUNCTION mangaale_online_delivery(kind text, qr boolean, md jsonb,
    creation text, dining bigint, counter bigint) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
    SELECT COALESCE(upper(trim(kind)) = 'DELIVERY'
        AND NOT COALESCE(qr, false)
        AND md->>'order_source' = 'customer_web'
        AND COALESCE(creation, '') IN ('', 'customer_web')
        AND dining IS NULL AND counter IS NULL, false)
$$;

CREATE OR REPLACE FUNCTION mangaale_dispatch_enabled(restaurant bigint) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT COALESCE((SELECT enabled AND
        (cardinality(restaurant_ids) = 0 OR restaurant = ANY(restaurant_ids))
        FROM online_delivery_dispatch_config WHERE singleton), false)
$$;

CREATE INDEX IF NOT EXISTS orders_online_delivery_reconcile_idx ON orders(created_at, order_id)
WHERE is_deleted = false AND metadata->>'order_source' = 'customer_web'
    AND order_status IN ('preparing', 'ready');

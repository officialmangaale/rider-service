-- Rider offer investigation — read-only queries.
--
-- Run every block inside a read-only transaction, e.g.
--   PGOPTIONS='-c default_transaction_read_only=on' psql "$DATABASE_URL" -X -f this_file
-- or wrap manually:
--   BEGIN TRANSACTION READ ONLY; ... ROLLBACK;
--
-- Parameters are psql variables; set them with -v, e.g.
--   -v order_id=13290 -v rider_user_id="'c6b4...'"
-- Nothing here writes. Output that would identify a person (phone, address,
-- exact coordinates) is never selected; coordinates are reported as presence,
-- distance and age only.

-- Q0. Session is read-only and the clock is sane.
SHOW default_transaction_read_only;
SELECT now() AS db_now_utc;

-- Q1. Recent delivery orders and their dispatch outcome (last 12 h).
SELECT o.order_id, o.restaurant_id, o.order_type, o.order_status,
       o.metadata->>'delivery_mode'           AS delivery_mode,
       (o.delivery_latitude IS NOT NULL)      AS has_drop_coords,
       COALESCE(o.assigned_rider_user_id,'') <> '' AS has_assigned_rider,
       o.created_at,
       d.delivery_order_id, d.delivery_status, d.assignment_type, d.restaurant_owned,
       d.created_at AS dispatched_at,
       (SELECT COUNT(*) FROM delivery_order_requests r WHERE r.delivery_order_id = d.delivery_order_id) AS offers
FROM orders o
LEFT JOIN delivery_orders d ON d.order_id = o.order_id
WHERE UPPER(TRIM(o.order_type)) = 'DELIVERY'
  AND COALESCE(o.is_qrunch, FALSE) = FALSE
  AND o.created_at >= (now() AT TIME ZONE 'UTC') - INTERVAL '12 hours'
ORDER BY o.created_at DESC;

-- Q2. Status history of one order.
SELECT status, changed_at, changed_by
FROM order_status_history WHERE order_id = :order_id ORDER BY changed_at;

-- Q3. Offers (delivery_order_requests) for one order.
SELECT r.request_id, r.rider_id, r.status, r.distance_km, r.expires_at,
       r.created_at, r.updated_at
FROM delivery_order_requests r WHERE r.order_id = :order_id ORDER BY r.created_at;

-- Q4. Online riders: availability, freshness and distance to a pickup
--     (distance only, rounded; no coordinates selected).
SELECT a.rider_id, a.is_online, a.is_available, a.current_order_id,
       l.last_updated_at, now() - l.last_updated_at AS location_age,
       ROUND((6371 * acos(LEAST(1, cos(radians(d.pickup_latitude)) * cos(radians(l.latitude))
             * cos(radians(l.longitude) - radians(d.pickup_longitude))
             + sin(radians(d.pickup_latitude)) * sin(radians(l.latitude)))))::numeric, 2) AS km_to_pickup
FROM rider_availability a
LEFT JOIN rider_locations l ON l.rider_id = a.rider_id
CROSS JOIN (SELECT pickup_latitude, pickup_longitude FROM delivery_orders WHERE order_id = :order_id) d
WHERE a.is_online
ORDER BY l.last_updated_at DESC NULLS LAST;

-- Q5. The target rider's identity rows across tables.
SELECT u.id, u.primary_role, u.status, u.is_available,
       (u.current_lat IS NOT NULL) AS users_has_location, u.last_location_update
FROM users u WHERE u.id::text = :rider_user_id;
SELECT * FROM rider_availability WHERE rider_id = :rider_user_id;
SELECT rider_id, last_updated_at, now() - last_updated_at AS age, is_deleted
FROM rider_locations WHERE rider_id = :rider_user_id;
SELECT restaurant_id, is_active, status FROM restaurant_riders WHERE rider_user_id = :rider_user_id;

-- Q6. The target rider's GPS history timestamps around the order (no coordinates).
SELECT recorded_at FROM rider_location_history
WHERE rider_id::text = :rider_user_id
  AND recorded_at BETWEEN (:from)::timestamptz AND (:to)::timestamptz
ORDER BY recorded_at;

-- Q7. Idempotency records for the order.
SELECT * FROM processed_events WHERE order_id = :order_id;  -- columns: see Q8

-- Q8. Schema introspection for the tables above.
SELECT table_name, column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name IN ('delivery_orders','delivery_order_requests','rider_availability',
                     'rider_locations','processed_events','restaurant_riders')
ORDER BY table_name, ordinal_position;
SELECT indexname, indexdef FROM pg_indexes
WHERE tablename IN ('delivery_orders','delivery_order_requests','rider_availability','rider_locations');

# Read-only checks used for order 13286

Every session ran with `PGOPTIONS='-c default_transaction_read_only=on'`, and
`SHOW default_transaction_read_only` returned `on`. No writes were made. Values
that identify people (phones, addresses, emails, names) are omitted.

## Order

```sql
SELECT order_id, restaurant_id, order_type, order_status, payment_status, pay_by,
       delivery_status, rider_id, assigned_rider_user_id,
       (delivery_latitude IS NOT NULL) AS has_drop_coords,
       created_at, updated_at, is_deleted
FROM orders WHERE order_id = 13286;
-- DELIVERY | confirmed | payment pending | cash | no rider | drop coords present

SELECT status, changed_at, changed_by FROM order_status_history
WHERE order_id = 13286 ORDER BY changed_at;
-- pending 06:05:49.69 system ; confirmed 06:06:03.16 owner

SELECT metadata->>'delivery_mode' FROM orders WHERE order_id = 13286;
-- restaurant_own_rider  (sent for platform dispatch: no live own rider)
```

## rider-service state

```sql
SELECT delivery_order_id, delivery_status, assignment_type, restaurant_owned,
       assigned_rider_id, created_at, updated_at
FROM delivery_orders WHERE order_id = 13286;
-- 16 | no_rider_found | platform | false | NULL | 06:06:03.198 | 06:06:03.204

SELECT * FROM delivery_order_requests WHERE delivery_order_id = 16;
-- 0 rows: no rider was ever offered the order

SELECT delivery_status, COUNT(*) FROM delivery_orders GROUP BY 1;
-- no_rider_found | 16   (every platform delivery order ever)
```

## Rider eligibility

```sql
SELECT a.rider_id, a.is_online, a.is_available, a.current_order_id,
       l.last_updated_at,
       <haversine km to pickup 28.4139, 77.0422> AS km
FROM rider_availability a
LEFT JOIN rider_locations l ON l.rider_id = a.rider_id
WHERE a.is_online;
-- R1: online, available, no order, 0.00 km, last GPS 06:13:11
-- 5 others: online flag set, GPS last seen in June

SELECT recorded_at FROM rider_location_history
WHERE rider_id = '<R1>' AND recorded_at > NOW() - INTERVAL '3 hours'
ORDER BY recorded_at;
-- 04:42:33, 04:44:54, 05:46:12, 05:50:20, 06:06:18, 06:09:41, 06:13:11
-- => 15m43s old at dispatch (06:06:03); the 5-minute rule excluded R1
```

## Re-dispatch candidate dry run

The exact `redispatchCandidateSQL` from `internal/repository/redispatch_repo.go`,
extracted from the source file and run read-only with max age 7200 s, cooldown
60 s, limit 50:

```
 delivery_order_id | order_id
-------------------+----------
                15 |    13283
                16 |    13286
```

The 13 older orders that still read `confirmed`/`preparing` (June, and 13173
from 2026-09-09) are correctly excluded by the 2-hour ceiling.

## Column nullability relied on

```sql
SELECT column_name, is_nullable FROM information_schema.columns
WHERE table_name = 'delivery_orders'
  AND column_name IN ('items_summary','customer_name','customer_phone', ...);
-- items_summary, customer_name, customer_phone: NOT NULL
-- restaurant_name/phone, assignment_type, restaurant_owned: nullable, 0 NULL rows
```

`GetDeliveryOrderByID` scans these into plain Go strings, so a NULL would fail the
load. There are none.

## HTTP probes (no credentials, rejected before any handler)

| Probe | Result | Meaning |
|---|---|---|
| `GET wss rider-prod /ws/rider` no token | 401 `token required` | route reaches rider-service |
| `GET rider-prod /ws/tracking/orders/0` with upgrade headers | **400**, `Sec-Websocket-Version: 13` | proxy drops `Upgrade`/`Connection` |
| `GET rider-prod /api/v1/riders/order-requests` no token | 401 | polling endpoint deployed |
| `POST restaurant-prod /internal/orders/0/assign-rider` no token | **503** `internal service authentication not configured` | `INTERNAL_SERVICE_TOKEN` unset in production |

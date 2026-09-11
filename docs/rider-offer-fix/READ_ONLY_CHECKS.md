# Read-only checks (production, 2026-09-11)

All run with `PGOPTIONS='-c default_transaction_read_only=on'`. No
INSERT/UPDATE/DELETE/DDL was run against production. Rider ids are
truncated; no phone, address or coordinates are reproduced. Full query set:
`docs/rider-offer-investigation/READ_ONLY_QUERIES.sql`.

| Check | Result |
|---|---|
| Recent confirmed delivery orders (12 h) | 13283, 13286, 13294, 13312 (+13279 completed without dispatch row) |
| Their `delivery_orders` | all `no_rider_found`, created within ~40 ms of confirmation |
| Offers (`delivery_order_requests`) for them | **0** (0 for every platform order ever) |
| Rider availability | 6 rows `is_online=true`; 5 have no location since June |
| Location freshness now | 0 riders fresh (< 5 min) |
| Target rider at 13294 dispatch (07:26:22 UTC) | online, available, idle, last fix 67 s old, 0.00 km → eligible |
| Old `FindNearestRiders` SQL | `ERROR: column "rl.rider_id" must appear in the GROUP BY clause…` |
| Corrected SQL (replayed at 07:26:22) | returns the target rider, 0.00 km |
| Final `nearestRidersSQL("")` and `…Among` strings, extracted from source | execute without error (0 rows: nobody fresh now); `EXPLAIN` (no ANALYZE) sane |
| Re-dispatch candidates at deploy time | **0 rows**: deploying offers nothing retroactively |

Queries for the final SQL were generated from the Go source with the
parameters substituted, e.g.:

```sql
-- FindNearestRiders, as shipped, with order 13294's pickup
SELECT rider_id, latitude, longitude, distance_km
FROM (
	SELECT rl.rider_id, rl.latitude, rl.longitude,
		(6371 * acos(LEAST(1.0, GREATEST(-1.0,
			cos(radians(<pickup_lat>)) * cos(radians(rl.latitude)) * cos(radians(rl.longitude) - radians(<pickup_lng>))
			+ sin(radians(<pickup_lat>)) * sin(radians(rl.latitude)))))) AS distance_km
	FROM rider_locations rl
	INNER JOIN rider_availability ra ON ra.rider_id = rl.rider_id
	WHERE ra.is_online = true AND ra.is_available = true AND ra.current_order_id IS NULL
	  AND rl.last_updated_at >= NOW() - INTERVAL '5 minutes'
) eligible_riders
WHERE distance_km <= 5.0
ORDER BY distance_km ASC
LIMIT 5;
```

Pre-deploy check: orders the `RedispatchWorker` would offer as soon as the
fixed build starts (`FindRedispatchCandidates`, 2 h window, 60 s cooldown).
Expect 0 rows, or knowingly accept that those orders get offered:

```sql
SELECT d.delivery_order_id, d.order_id
FROM delivery_orders d
JOIN orders o ON o.order_id = d.order_id
WHERE d.is_deleted = FALSE
  AND d.delivery_status IN ('pending', 'rider_searching', 'no_rider_found')
  AND d.assigned_rider_id IS NULL
  AND COALESCE(d.rider_user_id, '') = ''
  AND COALESCE(d.restaurant_owned, FALSE) = FALSE
  AND d.created_at >= NOW() - make_interval(secs => 7200)
  AND o.is_deleted = FALSE
  AND LOWER(o.order_status) IN ('accepted', 'confirmed', 'preparing', 'ready')
  AND COALESCE(o.assigned_rider_user_id, '') = ''
  AND NOT EXISTS (
        SELECT 1 FROM delivery_order_requests r
         WHERE r.delivery_order_id = d.delivery_order_id
           AND (r.status = 'accepted'
                OR r.expires_at > NOW() - make_interval(secs => 60)))
ORDER BY d.created_at ASC
LIMIT 50;
```

Integration tests ran on an isolated throwaway PostgreSQL 16 and Redis
(Unix sockets in a scratch directory, no TCP), never on production.

# Reproduction

Environment: production database, read-only (`default_transaction_read_only=on`).
No order, rider or dispatch row was created or changed.

## 1. The eligibility query fails on real PostgreSQL

Extract the exact query from `internal/repository/delivery_repo.go`
(`FindNearestRiders`), substitute order 13294's pickup, run read-only:

```bash
python3 - > /tmp/nearest.sql <<'EOF'
import re
s=open('internal/repository/delivery_repo.go').read()
i=s.index('func (r *DeliveryRepository) FindNearestRiders')
q=re.search(r'query := `(.*?)`', s[i:], re.S).group(1)
q=(q.replace('$1',"(SELECT pickup_latitude FROM delivery_orders WHERE order_id=13294)")
    .replace('$2',"(SELECT pickup_longitude FROM delivery_orders WHERE order_id=13294)")
    .replace('$3','5.0').replace('$4','5'))
print(q+';')
EOF
PGOPTIONS='-c default_transaction_read_only=on' psql "$DATABASE_URL" -X -f /tmp/nearest.sql
```

Result (2026-09-11 10:4x UTC):

```
ERROR:  column "rl.rider_id" must appear in the GROUP BY clause or be used in an aggregate function
```

Independent of data: the error is raised during query analysis.

## 2. The rider was eligible at 13294's dispatch instant

Same filters, distance test in `WHERE`, `NOW()` pinned to 07:26:22.087 UTC:

```sql
WITH p AS (SELECT pickup_latitude AS lat, pickup_longitude AS lng FROM delivery_orders WHERE order_id = 13294),
     at AS (SELECT TIMESTAMPTZ '2026-09-11 07:26:22.087+00' AS t)
SELECT left(rider_id,8)||'…' AS rider, ROUND(distance_km::numeric,2) AS km
FROM (
  SELECT rl.rider_id,
         (6371 * acos(LEAST(1.0, cos(radians(p.lat)) * cos(radians(rl.latitude)) * cos(radians(rl.longitude) - radians(p.lng))
                          + sin(radians(p.lat)) * sin(radians(rl.latitude))))) AS distance_km
  FROM rider_locations rl
  JOIN rider_availability ra ON ra.rider_id = rl.rider_id
  CROSS JOIN p CROSS JOIN at
  WHERE ra.is_online AND ra.is_available AND ra.current_order_id IS NULL
    AND rl.last_updated_at >= at.t - INTERVAL '5 minutes'
    AND rl.last_updated_at <= at.t
) c
WHERE distance_km <= 5.0
ORDER BY distance_km LIMIT 5;
```

Result: `c6b46748… | 0.00`.

Valid because `rider_locations` holds the latest fix (07:25:15) and no
later fix exists before 10:00 (Q6), and `rider_availability.updated_at`
(09-09 17:17) predates the order.

## 3. Production logs (operator)

Container logs were not accessible from the investigation environment.
On the server:

```bash
docker logs <rider-service-container> --since 2026-09-11T07:00:00Z 2>&1 \
  | grep -E 'Nearest rider search failed|REDISPATCH-WORKER|Consumed ORDER_PLACED order_id=13294'
```

Expected: `[DELIVERY] Nearest rider search failed for order 13294: pq: column "rl.rider_id" must appear …`.

## 4. Controlled post-fix reproduction (to run after the fix is approved)

Preconditions: staging or a controlled production test restaurant; one
self-signup rider, approved, Online, app in the **foreground** (background
mode not yet deployed), socket connected (no "Reconnecting live orders…").

1. Enable the trace for that rider (OBSERVABILITY_PLAN.md).
2. Customer places a Delivery order within 5 km of the rider.
3. Owner accepts.
4. Fill TRACE_LEDGER.md Trace C from the `[DISPATCH]` lines.
5. Expected: `dispatch.eligibility.evaluated result=ok eligible_count≥1`,
   `dispatch.eligibility.target_rider eligible=true selected_by_search=true`,
   `dispatch.offer.persisted`, `websocket.offer.lookup connections=1 result=enqueued`;
   card and a single ring on the device.

Negative cases (expected reason in `[DISPATCH]` output):

| Case | Expected |
|---|---|
| rider Offline | `target_rider first_failure=rider_not_online` |
| stale location (app backgrounded > 5 min) | `first_failure=rider_location_stale`, `location_age_s>300` |
| outside radius | `first_failure=rider_outside_radius` |
| rider on another order | `first_failure=rider_busy` |
| socket disconnected | `websocket.offer.lookup result=not_sent reason_code=no_matching_connection`; card appears within 10 s via poll |
| offer ignored | `ORDER_REQUEST_EXPIRED` after 30 s; re-offer by `RedispatchWorker` after 60 s |
| second rider accepts | loser gets `ORDER_ASSIGNED_TO_OTHER_RIDER` |
| unapproved rider | cannot go Online (app `_ensureProfileReadyForOnline`); dispatch has no approval filter |

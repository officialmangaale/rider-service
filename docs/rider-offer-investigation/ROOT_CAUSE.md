# Root cause — confirmed delivery orders never become rider offers

## 1. Symptom

A restaurant confirms a delivery order; the customer app shows it confirmed;
an Online self-signup rider never sees an offer, and nothing rings.

## 2. Scope

Every platform (non-restaurant-owned) delivery order since the dispatch code
was written. Production: all 18 `delivery_orders` rows ever created are
`no_rider_found`; `delivery_order_requests` (offers) has **never** held a row
for them. Rider-app version is irrelevant: no offer ever existed to deliver.

## 3. First failed boundary

**Rider eligibility search**, rider-service:

- repository `rider-service`
- file `internal/repository/delivery_repo.go`
- symbol `(*DeliveryRepository).FindNearestRiders`
- condition: the SQL filters distance with `HAVING (<distance>) <= $3` and has
  no `GROUP BY`, while selecting the plain columns `rl.rider_id, rl.latitude,
  rl.longitude`.

PostgreSQL rejects this at query analysis, before reading any row:

```
ERROR:  column "rl.rider_id" must appear in the GROUP BY clause or be used in an aggregate function
```

The caller `DeliveryService.ProcessOrderPlacedEvent`
(`internal/service/delivery_service.go`) treats the error like "nobody
nearby": logs one free-text line, sets `delivery_orders.delivery_status =
'no_rider_found'`, and returns. No offer is written, so nothing is published to
the rider socket and nothing is available to the app's pending-requests poll.

## 4. Classification

| Statement | Class |
|---|---|
| `FindNearestRiders` fails on every call in production Postgres | **proven** (exact SQL from source, run read-only on production, error above) |
| This is why orders 13294 and 13312 got no offer | **proven** for 13294 (target rider passed every filter at dispatch time, below); **proven** for 13312 that the search failed (the error is unconditional); the rider was also stale for 13312 |
| The same defect affected 13283 and 13286 | **proven** (unconditional error; same `no_rider_found` in ≤ 5 ms, zero offers) |
| Stale rider location was the cause of 13286 (earlier report, docs/rider-dispatch) | **ruled out as root cause**; it was a real, additional condition for 13286, but the search could not succeed regardless |
| The Redis fast path could have produced an offer | **ruled out** (§7) |
| WebSocket delivery or the rider app lost an offer | **not applicable yet**: no offer was ever created; those boundaries were never exercised in production |

## 5. Evidence

### Order 13294 (read-only, production)

| Fact | Value | Source |
|---|---|---|
| Owner confirmed | 07:26:22.087 UTC, `changed_by=owner` | `order_status_history` (Q2) |
| Event consumed | `ORDER_PLACED:13294` processed 07:26:22.126 | `processed_events` (Q7) |
| Dispatch row | `delivery_orders.delivery_order_id=17`, created 07:26:22.124, `no_rider_found` at 07:26:22.129 (5 ms) | Q1 |
| Offers | 0 rows | `delivery_order_requests` (Q3) |
| Target rider `c6b46748…` | `rider_availability`: online, available, `current_order_id` NULL; `rider_locations.last_updated_at` 07:25:15 (67 s before dispatch, inside the 5-minute window); 0.00 km from pickup; no `restaurant_riders` link (self-signup); `users.primary_role=delivery_driver`, `status=active` | Q4, Q5, Q6 |

Replay: the same filters with the distance test moved from `HAVING` to
`WHERE`, and `NOW()` pinned to 07:26:22.087, return exactly that rider at
0.00 km (query in REPRODUCTION.md). So under the existing rules the rider
was eligible, and only the SQL defect stopped the offer.

### The defect itself

`git log -S'HAVING (6371'` → introduced in `f91c24d` (2026-05-09), present in
every deployed build since. The Go code compiles and every unit test passes,
because the only tests that touch this query use `sqlmock`, which matches the
SQL by regex and returns fabricated rows. It was never executed against
PostgreSQL in any test.

## 6. Why existing logs and tests did not expose it

- The error is logged once per order as `[DELIVERY] Nearest rider search
  failed for order N: pq: …`, but the persisted outcome, `no_rider_found`, is
  identical to "no rider nearby". Everyone reading the database (including
  the earlier investigation) saw a normal-looking outcome.
- No metric or alert exists; rider-service has no metrics stack.
- The eligibility funnel (`GetRiderEligibilitySummary`) was logged only on the
  zero-riders path, never on the error path, so "1 rider would qualify" was
  never printed next to the failure.
- Tests mock SQL (`sqlmock`) and so cannot detect a query PostgreSQL rejects.

## 7. Contributing factors (not root cause)

1. **Redis fast path is never fed by the app.** `RedisDispatchCache.FindNearestRiders`
   (`internal/cache/redis_dispatch.go`) requires a fresh `locationUpdatedKey`
   entry, written only by `DeliveryService.UpdateRiderLocation`, i.e. route
   `POST /api/v1/riders/location`. The rider app uploads to
   `POST /api/v1/location/update` (`LocationService.UpdateLocation`), which
   does not touch Redis. With `REDIS_URL` set, Redis returns nobody and the
   code falls through to the failing SQL. Had Redis been fed, offers would
   have been created via Redis and this defect would have been masked.
2. **Location goes stale while the app is backgrounded** (single GPS fix
   07:25:15 between 07:15 and 10:00). For 13312 (09:52) the rider was 2 h 27 m
   stale and would have been excluded even with a working query. Addressed
   separately by the rider background-mode work (not yet deployed).
3. **SQS FIFO de-duplication** uses the stable id `ORDER_PLACED:<order_id>`
   (`restaurant-service/internal/events/publisher.go`), so a second publish
   within the FIFO window (e.g. confirmed → preparing a minute later) is
   dropped by SQS. Re-dispatch now relies on the rider-service
   `RedispatchWorker` rather than on re-published events.

## 8. Correction — applied 2026-09-11 (see docs/rider-offer-fix/)

Applied as proposed below, plus: the Redis fast path now only proposes
candidates that PostgreSQL re-checks, the app's location route indexes Redis,
and concurrent accepts no longer deadlock. Verified on a real PostgreSQL.

Original proposal:

Change only the SQL shape of `FindNearestRiders`; keep every filter, the
5-minute window, radius, ordering and limit:

```go
query := `
	SELECT rider_id, latitude, longitude, distance_km FROM (
		SELECT rl.rider_id, rl.latitude, rl.longitude,
			(6371 * acos(
				LEAST(1.0, cos(radians($1)) * cos(radians(rl.latitude)) * cos(radians(rl.longitude) - radians($2))
				+ sin(radians($1)) * sin(radians(rl.latitude)))
			)) AS distance_km
		FROM rider_locations rl
		INNER JOIN rider_availability ra ON ra.rider_id = rl.rider_id
		WHERE ra.is_online = true
		  AND ra.is_available = true
		  AND ra.current_order_id IS NULL
		  AND rl.last_updated_at >= NOW() - INTERVAL '5 minutes'
	) candidates
	WHERE distance_km <= $3
	ORDER BY distance_km ASC
	LIMIT $4`
```

Tests for the correction:

- a repository test pinning the query shape (no `HAVING`; distance filtered
  in `WHERE`), since sqlmock cannot run it;
- the corrected SQL executed read-only against production (done for the
  replay; repeat on the final string);
- recommended: a CI job or build-tagged integration test against a real
  PostgreSQL (e.g. docker `postgres:18`) that runs every repository query
  once. This class of bug cannot be caught any other way.

Compatibility and security: the query returns the same columns in the same
order; no API, schema or permission change.

Rollout: deploy rider-service; place one controlled order (REPRODUCTION.md);
expect `dispatch.eligibility.evaluated result=ok eligible_count=1`, then
`dispatch.offer.persisted` and `websocket.offer.lookup`. Rollback: redeploy
the previous image (behaviour returns to "no offers", i.e. today's state).

Expected side effect once fixed: the `RedispatchWorker` will begin offering
unassigned orders confirmed within the last 2 hours (13294 was still
`confirmed` at investigation time; it falls outside the window after
09:26 UTC). Check for such orders before deploying.

## 9. Remaining unknowns

- Production container logs were not accessible here. The line
  `[DELIVERY] Nearest rider search failed for order 13294: pq: column "rl.rider_id" …`
  should be present; see REPRODUCTION.md for the command.
- Whether the deployed rider-service image includes the `RedispatchWorker`
  (commit `aba4cd1`). If it does, its log shows the same SQL error every 20 s
  per candidate order.
- The WebSocket and app boundaries remain unproven in production because no
  offer has ever existed. They are instrumented now and must be verified in
  the first post-fix trace.

# Trace ledger

## Trace A — order 13294 (historical, production, read-only)

Target: `TARGET_ORDER_ID=13294`, `TARGET_RESTAURANT_ID=27`,
`TARGET_RIDER_USER_ID=c6b46748-16e5-443a-8daf-c82ffe6130c4` (auth `sub` =
rider key everywhere; see IDENTITY_MAP.md), window 07:15–07:35 UTC.

| UTC | Service | IDs | Checkpoint | Result | Reason / error | Evidence |
|---|---|---|---|---|---|---|
| 07:25:15 | rider-app → rider-service | rider c6b4… | last location upload before dispatch | pass | — | `rider_locations.last_updated_at`, `rider_location_history` (Q5/Q6) |
| 07:26:09.247 | restaurant-service | order 13294 | order created `pending` | pass | — | `order_status_history` (Q2) |
| 07:26:22.087 | restaurant-service | order 13294 | owner accept committed `confirmed` | **pass** | — | Q2 `changed_by=owner` |
| 07:26:22.1xx | restaurant-service | event `ORDER_PLACED:13294` | dispatch trigger + SQS publish | **pass** | — | consumed below (publisher log not accessible) |
| 07:26:22.124 | rider-service | delivery_order 17 | `delivery_orders` row created (platform) | **pass** | — | Q1 |
| 07:26:22.126 | rider-service | `ORDER_PLACED:13294` | event marked processed | pass | — | `processed_events` (Q7) |
| 07:26:22.12x | rider-service | order 13294 | own-rider gate | pass (not held) | — | status reached `no_rider_found`, not `rider_searching` |
| 07:26:22.12x | rider-service | order 13294 | Redis GEO search | returns none | Redis never fed by `/location/update` | ROOT_CAUSE §7 |
| 07:26:22.12x | rider-service | order 13294 | **SQL eligibility search** | **FAIL** | `column "rl.rider_id" must appear in the GROUP BY clause…` | exact SQL run read-only (REPRODUCTION.md) |
| 07:26:22.129 | rider-service | delivery_order 17 | status → `no_rider_found` | executed | error path | Q1 `updated_at` |
| — | rider-service | — | offer persisted | **not reached** | — | Q3: 0 rows |
| — | rider-service | — | socket publish / write | not reached | — | — |
| — | rider-app | — | frame / card / ringtone | not reached | — | — |

Counterfactual for the failed row: with the distance test in `WHERE`, rider
c6b4… is returned at 0.00 km for 07:26:22.087 (REPRODUCTION.md §2). Every
eligibility filter passed: online ✔ available ✔ idle ✔ location present ✔
fresh (67 s) ✔ within 5 km ✔.

## Trace B — order 13312 (historical)

| UTC | Checkpoint | Result | Evidence |
|---|---|---|---|
| 09:52:17.441 | owner confirmed | pass | Q2 |
| 09:52:17.477 | delivery row 18 created | pass | Q1 |
| 09:52:17.479 | event processed | pass | Q7 |
| 09:52:17.482 | eligibility search | **FAIL** (same SQL error; unconditional) | REPRODUCTION.md §1 |
| — | target rider | also ineligible: location 2 h 27 m stale | Q4 |
| 09:57:12 | owner completed the order without a rider | — | Q2 |

## Trace C — first post-fix controlled order (to fill)

Enable the trace (OBSERVABILITY_PLAN.md), then record each line:

| UTC | Service | event | key fields | Result |
|---|---|---|---|---|
| | rider-service | `websocket.connection.opened` | rider_id, conn_id | |
| | restaurant-service | `events: publishing delivery order for rider dispatch` | order_id | |
| | rider-service | `dispatch.attempt.started` | order_id, event_id | |
| | rider-service | `dispatch.eligibility.evaluated` | result, eligible_count | |
| | rider-service | `dispatch.eligibility.target_rider` | first_failure, selected_by_search | |
| | rider-service | `dispatch.offer.persisted` | request_id | |
| | rider-service | `websocket.offer.lookup` | connections, enqueued, result | |
| | rider-app | debug `[RiderSocket] event type=DELIVERY_ORDER_REQUEST` | — | |
| | rider-app | card visible + one ring | — | |

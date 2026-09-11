# Rider offer fix — investigation and changes (2026-09-11)

Evidence in detail: `docs/rider-offer-investigation/ROOT_CAUSE.md`.
Note: the offer table is `delivery_order_requests` (there is no
`delivery_offers` table).

## Where the flow stopped

| Boundary | Code | Before | After |
|---|---|---|---|
| Owner accepts → dispatch trigger | restaurant-service `routes.go` `handleOrderStatusChanged` → `publishDeliveryOrderForDispatch` → SQS `ORDER_PLACED` | works | unchanged |
| Consume → delivery row | `worker/sqs_consumer.go` → `DeliveryService.ProcessOrderPlacedEvent` → `delivery_orders` | works | unchanged |
| **Rider search** | `repository/delivery_repo.go` `FindNearestRiders` | **SQL rejected by PostgreSQL on every call** → `no_rider_found` | fixed |
| Redis fast path | `cache/redis_dispatch.go` | never populated by the app's location route; availability never written by go-online/accept/delivery | candidates only, re-checked in PostgreSQL |
| Offer persisted | `sendOffers` → `CreateRequest` → `delivery_order_requests` | never reached | reached (tested) |
| Socket publish | `ws.Hub.SendToRiderCount` | never reached | reached; zero-connection case logged |
| Poll recovery | `GET /api/v1/riders/order-requests` → `GetPendingRequestPayloads` | never had rows | returns the offer (tested) |
| Accept | `AcceptRequest` | exclusive, but simultaneous accepts deadlocked (1 s, loser got an internal error) | lock order fixed; loser told "order already assigned to another rider" |

## Routes checked

| Route | Handler → service | Writes | Redis |
|---|---|---|---|
| `POST /api/v1/location/update` (rider app) | `LocationHandler.UpdateLocation` → `LocationService.UpdateLocation` | `users`, `rider_locations`, `rider_location_history` | **now indexed** after PostgreSQL succeeds |
| `POST /api/v1/riders/location` | `DeliveryHandler.UpdateLocation` → `DeliveryService.UpdateRiderLocation` | `rider_locations` | indexed (same `IndexRiderLocation`) |
| `POST /api/v1/rider/go-online` / `go-offline` (rider app) | `RiderService.GoOnline/GoOffline` | `users.is_available`, `rider_availability` | none (not needed: Redis no longer decides eligibility) |
| `POST /api/v1/riders/availability` | `DeliveryService.UpdateRiderAvailability` | `rider_availability`, `users` | availability hash (unused by search now) |

The two location routes keep their own PostgreSQL writes. Full unification
into one method was not done: `/riders/location` also broadcasts
`RIDER_LOCATION_UPDATED` to the **unauthenticated** order-tracking socket
(`/ws/tracking/orders/:id`), and routing the app's main location stream
through that would widen who can see a rider's coordinates. Both routes now
share the dispatch-relevant parts: `rider_locations` then `IndexRiderLocation`.

## Changes (rider-service)

| File | Change |
|---|---|
| `internal/repository/delivery_repo.go` | `nearestRidersSQL`: distance in a subquery, radius in the outer `WHERE` (not `GROUP BY`); `acos` clamped with `LEAST/GREATEST`; `FindNearestRidersAmong` (same rules, candidate list); `rows.Err()` checked; `LockDeliveryOrderForRequest` |
| `internal/cache/redis_dispatch.go` | `NearbyCandidateIDs` (GEO + fresh timestamp) replaces `FindNearestRiders`, which required Redis availability that most paths never wrote |
| `internal/service/delivery_service.go` | `findOfferableRiders` → `redisCandidateRiders` (Redis proposes, PostgreSQL decides, SQL when Redis does not fill the list); `IndexRiderLocation`; accept locks the delivery order first; cancelled offer → "order already assigned to another rider"; failed withdraw is returned, not ignored; accept/decline/withdraw events |
| `internal/service/location_service.go` | indexes the location after the PostgreSQL write (`SetLocationIndexer`) |
| `internal/router/router.go` | `locationSvc.SetLocationIndexer(deliverySvc)` |
| `internal/worker/expiry_worker.go` | `dispatch.offer.expired` event |
| `internal/dispatchtrace/dispatchtrace.go` | new event names / reason codes |
| `internal/testpg/testpg.go` | real-PostgreSQL test harness |
| tests | see DEPLOYMENT_CHECKLIST.md "Verification" |

No schema change, no migration, no API contract change.

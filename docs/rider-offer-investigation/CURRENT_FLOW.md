# Current flow — restaurant accept to rider offer

Verified in code on 2026-09-11. Terms: **order** (customer order),
**offer** (`delivery_order_requests` row, time-bounded, per rider),
**assignment** (exclusive accept result on `delivery_orders.assigned_rider_id`).

## A. Restaurant acceptance (restaurant-service)

| Step | Code |
|---|---|
| Owner taps Accept | restaurant-owner `lib/services/order_service.dart:313` → `PUT {restaurantBaseUrl}/orders/:id/status` |
| Route | `restaurant-service/routes/routes.go:602` `mgmtOrders.PUT("/:id/status", orderC.UpdateStatus)` |
| Commit hook | `orderSvc.SetOnOrderStatusChanged(handleOrderStatusChanged)` (`routes.go:982`); `handleOrderStatusChanged` (`routes.go:819`) runs after the status commit, publishes KDS/owner/customer realtime first |
| Dispatch trigger | `routes.go:935–939`: for `accepted/confirmed/preparing/ready` → `publishDeliveryOrderForDispatch(orderID, restaurantID, "order_status_<status>")` |
| Guards | `routes.go:1755+`: order load ok; `events.IsDeliveryOrder`; no own rider assigned; status dispatchable; own-rider decision `services.DecideOwnRiderDispatch` (hold vs platform); each branch logged with the structured `logger` |
| Publish | `eventPublisher.PublishOrderPlaced` (`internal/events/publisher.go:33`): goroutine, SQS FIFO, `event_id = ORDER_PLACED:<order_id>` (also the FIFO dedup id), retry, logs publish and failure |
| Other triggers | order creation (`routes.go:1962`), own-rider hold timeout worker (`routes.go:1994`) |

Customer app status comes from the same commit hook (`orderLiveBroadcaster`),
independent of dispatch, which is why the customer and owner apps update
correctly while dispatch fails.

## B. Dispatch initiation (rider-service)

| Step | Code |
|---|---|
| Consumer | `internal/worker/sqs_consumer.go:166–183` → `DeliveryService.ProcessOrderPlacedEvent` |
| Idempotency | `processed_events` by event id and order id; `canRedispatch` for repeats |
| Delivery row | `CreateDeliveryOrder` → `delivery_orders` (`UNIQUE(order_id)`), status `rider_searching`, `assignment_type='platform'` |
| Own-rider gate | `requiresOwnRiderCheck(delivery_mode)`: only an explicit restaurant-owned mode is re-checked (`HasActiveRestaurantOwnRiders`) |
| Search | `findOfferableRiders` → declined-rider exclusion → Redis GEO (`internal/cache/redis_dispatch.go`) → SQL `DeliveryRepository.FindNearestRiders` (**fails**, ROOT_CAUSE.md) |
| Error path | log + `delivery_status='no_rider_found'` + return nil |
| Zero riders | eligibility funnel + `no_rider_found` + `DELIVERY_STATUS_UPDATED` to order tracking socket |
| Re-dispatch | `internal/worker/redispatch_worker.go` every `REDISPATCH_INTERVAL_SECONDS` (20) over `FindRedispatchCandidates` → `RedispatchOrder` → same search |

## C. Eligibility rules (`FindNearestRiders`, SQL)

A rider is a candidate only if **all** hold:

| # | Rule | Column |
|---|---|---|
| 1 | availability row exists | `rider_availability.rider_id` (joined on `rider_locations.rider_id`) |
| 2 | Online | `rider_availability.is_online = true` |
| 3 | Available | `rider_availability.is_available = true` |
| 4 | Idle | `rider_availability.current_order_id IS NULL` |
| 5 | Location exists | `rider_locations` row |
| 6 | Fresh | `rider_locations.last_updated_at >= NOW() - 5 minutes` |
| 7 | Within radius | haversine km ≤ `SEARCH_RADIUS_KM` (default 5; unset in `.env` → 5) |
| 8 | Not declined this order | `delivery_order_requests.status='rejected'` excluded (Go) |

Not checked by dispatch: KYC/documents, vehicle type, city/zone, shift,
suspension (`users.status`), device registration. Onboarding is enforced
when the rider goes Online (rider-app `_ensureProfileReadyForOnline`), not at
dispatch. Nearest first, `LIMIT MAX_RIDERS_TO_NOTIFY` (default 5).

## D. Offer creation

`sendOffers` → `CreateRequest` (`INSERT … ON CONFLICT (delivery_order_id,
rider_id) DO UPDATE … WHERE status IN ('rejected','expired','cancelled')`),
`expires_at = now + REQUEST_EXPIRY_SECONDS` (30). Persisted before publish;
a publish failure leaves the offer readable by the poll. `ExpiryWorker`
(10 s) expires offers and sends `ORDER_REQUEST_EXPIRED`.

## E. Publication and socket

- Hub `internal/ws/hub.go`, in-process map `riderClients[rider_id]`; key is
  the JWT `sub` from `/ws/rider?token=…` (`HandleRiderWS`). One replica; no
  cross-instance fan-out (no Redis pub/sub for sockets).
- `SendToRider` → per-client buffered channel (256) → `writePump` (10 s
  write deadline, 30 s ping); `readPump` 60 s read deadline.
- Proxy: rider-prod nginx `/ws/` location with `Upgrade`/`Connection`
  headers (added 2026-09-11; probe returns 101).

## F. Rider-app receive path

- URL `AppConstants.riderWsUrl = wss://rider-prod.mangaale.com/ws/rider`,
  token as query parameter read on every connect (`RiderSocketService`).
- Events handled: `DELIVERY_ORDER_REQUEST`, `ORDER_REQUEST_EXPIRED`,
  `ORDER_ASSIGNED_TO_OTHER_RIDER`, assignment variants
  (`rider_socket_service.dart _handleMessage`).
- Recovery: `GET /api/v1/riders/order-requests` on connect, resume and every
  10 s while the socket is down (`_startFallbackPolling`).
- Card: dashboard `ref.listen(pendingRequests)` → `IncomingOrderRequestSheet`.
- Ring: `_alertNewOffers` (shared offer key across socket/poll/service).
- "Reconnecting live orders…" = `isOnline && !socketConnected`
  (`dashboard_screen.dart`). Its earlier permanent state came from the proxy
  rejecting the upgrade (400), fixed 2026-09-11.

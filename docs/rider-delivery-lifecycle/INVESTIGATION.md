# Rider delivery lifecycle — investigation (2026-09-11)

Symptoms: after a rider accepts, new_user_app keeps showing "Finding a
delivery partner"; the rider can press "I reached restaurant" but the next
step fails with "Could not update order. Refreshing the valid next step."

## Baseline

All five repos clean at start (rider-service `53b80d8`, rider-app `6bb944d`,
new_user_app `8cd8dbb`, restaurant-service `71249bd` on `production`,
restaurant-owner `ebdf6e9`). During the work, unrelated uncommitted changes
appeared in rider-app (app icons/branding, AndroidManifest, pubspec, settings,
splash, navigation_widgets); none of those files were touched.

## Root causes (proven)

### 1. Every rider-service → restaurant-service callback returned 404

`RESTAURANT_SERVICE_INTERNAL_BASE_URL=https://restaurant-prod.mangaale.com/`
(trailing slash, present since the tracked `.env` was created on 2026-08-10).
`client.RestaurantClient` builds `"%s/internal/orders/%d/…"`, so requests go
to `//internal/orders/13356/assign-rider`. restaurant-prod's nginx uses
`proxy_pass http://127.0.0.1:8082;` (no URI part → path passed unchanged),
and restaurant-service's gin router (v1.11, default settings) answers 404.

- Reproduced locally with gin v1.11: single slash → reaches the route (401
  without token); double slash → **404**.
- Production evidence (read-only): order 13356 — rider-service assigned rider
  `c6b46748…` at 12:15:55 and recorded `rider_arrived_restaurant` at 12:16:16,
  but restaurant-service's `orders` row has **no rider at all**
  (`assigned_rider_user_id`, `rider_id`, `delivery_partner_id` empty,
  `delivery_status=pending`). In the last 60 days **no** delivery order has a
  rider recorded on `orders`.
- The same prefix breaks `delivery-status`, so `picked_up` always failed:
  rider-service calls restaurant-service first and aborts on its error.

Consequences:
- Customer: `/customer-web/orders/:id/track` reads the rider from `orders`
  → `rider: null` → "Finding a delivery partner" forever.
- Rider: "I reached restaurant" is rider-service-only, so it worked; "Picked
  up" needs restaurant-service → 404 → "Could not update order…". This is the
  "one step then stuck" pattern. The owner moving the order to Preparing was
  coincidental, not the cause (13356 was already `ready` at 12:17:33).

Not verified: production container logs (no access). The rider-service log
should show `[RESTAURANT-CLIENT] Attempt 1/3 failed for order 13356:
restaurant-service returned status 404`. A missing `INTERNAL_SERVICE_TOKEN` on
restaurant-service would give 503 instead and needs the same deploy check.

### 2. Nothing repaired a failed assignment callback

Acceptance fired the callback asynchronously, 3 attempts, then gave up
("assignment kept locally"). From then on restaurant-service rejected every
canonical step from that rider (`403 rider is not assigned to this order`).

### 3. A rider on an order the owner closed stays busy forever

Order 13356 was completed by the owner at 12:50:05 while its delivery was at
`rider_arrived_restaurant`. rider-service never noticed:
`rider_availability.current_order_id=13356` still. Dispatch requires
`current_order_id IS NULL`, so this rider was invisible to every later order —
13360 and 13374 got `no_rider_found` with zero offers.

### 4. Kitchen prerequisite (a rule, not a bug)

restaurant-service lets a rider move an order only `ready → out_for_delivery →
delivered` (`services/order_status_contract.go`). A pickup while the order is
`confirmed`/`preparing` is refused with 400. The rule is kept (the kitchen
releases the food; prep timers auto-advance to ready), but the rider app
previously showed the pickup button anyway and a generic error.

### 5. Smaller gaps found on the way

- Rider active delivery omitted `customer_name`, `customer_phone`,
  `items_summary` although `delivery_orders` has them (the shared-orders path
  always sent them).
- Retrying the current step (lost response) failed with "invalid transition".
- restaurant-service's refusal reason was discarded (status code only).
- new_user_app ignored `delivery_status` and drew `ready` as "Rider assigned"
  even with no rider; assignment is not pushed on the customer socket
  (restaurant-service's `broadcastOrderProjection` publishes to the restaurant
  channel only), so the app learnt of it only via its 15 s poll.
- Rider app: a single context-dependent Navigate button, none at
  `rider_arrived_restaurant`; no separate pickup/drop cards.

## Boundary trace

| # | Step | Endpoint / code | Tables | Status written | Event |
|---|---|---|---|---|---|
| 1 | Customer places order | restaurant-service customer-web | orders | order `pending` | — |
| 2–3 | Owner accepts | restaurant-service owner route | orders, order_status_history | `confirmed` | customer + restaurant WS; SQS `ORDER_PLACED` |
| 4 | Delivery created | rider-service `ProcessOrderPlacedEvent` | delivery_orders | `rider_searching` | — |
| 5 | Offers | `sendOffers` | delivery_order_requests | request `pending` | rider WS `DELIVERY_ORDER_REQUEST`; poll `GET /api/v1/riders/order-requests` |
| 6–8 | Rider accepts | `POST /api/v1/riders/order-requests/:id/accept` → `AcceptRequest` | delivery_order_requests, delivery_orders, rider_availability, users | request `accepted`, others `cancelled`; delivery `rider_assigned` | rider WS `ORDER_ASSIGNED_TO_OTHER_RIDER` to others |
| 9 | Rider recorded for customer | `POST {restaurant}/internal/orders/:id/assign-rider` (async + retries; repaired on next step) | orders | orders.delivery_status `rider_assigned`, rider fields | restaurant WS `ORDER_DETAILS_UPDATED` (not customer) |
| 10 | Rider sees active delivery | `GET /api/v1/orders/active` → `OrderService.GetActiveOrder` | delivery_orders, orders (read) | — | — |
| 11 | Reached restaurant | `POST /api/v1/riders/orders/:orderId/status` `rider_arrived_restaurant` | delivery_orders, delivery_status_history | `rider_arrived_restaurant` | order-tracking WS `DELIVERY_STATUS_UPDATED` |
| 11 | Picked up | same, `picked_up` → `internal/orders/:id/delivery-status` | orders (canonical `out_for_delivery`), delivery_orders | `picked_up` | customer WS `ORDER_STATUS_UPDATED` |
| 11 | On the way | same, `on_the_way` | orders.delivery_status `out_for_delivery` | `on_the_way` | — (same order status) |
| 12–13 | Delivered | same, `delivered` (+`payment_collected` for cash) | orders `delivered`, delivery_orders, rider_availability, rider_earnings | `delivered` | customer WS |
| — | Owner closes mid-delivery | `ClosedDeliveryWorker` (30 s) / next status update | delivery_orders, rider_availability, delivery_status_history | delivery `cancelled` | — |

Customer app: `/tracking/:id` → `GET /customer-web/orders/:id/track`
(order_status, delivery_status, rider{name, phone, vehicle, live location});
socket `wss://…/ws/orders/status?order_id=` → any message refetches.

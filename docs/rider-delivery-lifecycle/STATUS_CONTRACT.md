# Delivery status contract

Four domains, related but not the same. None was renamed.

## A. Restaurant order status — `orders.order_status` (restaurant-service owns)

`pending → confirmed → preparing → ready → out_for_delivery → delivered → completed`,
plus `cancelled`, `rejected`. Actors (`services/order_status_contract.go`):

| Actor | Allowed |
|---|---|
| owner/staff | any (operational override) |
| KDS | confirmed→preparing, preparing→ready, ready→completed (non-delivery) |
| rider | **ready → out_for_delivery**, **out_for_delivery → delivered** (delivery orders) |

## B. Delivery status — `delivery_orders.delivery_status` (rider-service owns)

```
rider_searching / no_rider_found   (dispatch)
rider_assigned                      accept (assigned and accepted are one state)
  → rider_arrived_restaurant        rider-service only
  → picked_up                       needs A = ready (becomes out_for_delivery)
  → on_the_way                      A stays out_for_delivery
  → delivered                       A → delivered; cash needs payment_collected
cancelled                           owner closed the order mid-delivery (new use)
```

Restaurant-owned deliveries may go `rider_assigned|rider_arrived_restaurant → picked_up`
and `picked_up → delivered` directly (unchanged).

Rules (rider-service `UpdateDeliveryStatus`):

| Rule | Result | `error_code` |
|---|---|---|
| no delivery for order | 409 | `DELIVERY_NOT_FOUND` |
| caller not the assigned rider | 409 | `NOT_ASSIGNED_RIDER` |
| same status as current (retry) | **200, no side effects** (new) | — |
| owner completed/cancelled/rejected the order | 409, rider released (new) | `ORDER_CLOSED` |
| skipped or backwards step | 409 | `INVALID_TRANSITION` (+ `expected_status` in log) |
| cash order delivered without confirmation | 400 | `CASH_COLLECTION_REQUIRED` |
| pickup while A ∉ {ready, out_for_delivery} | 409, restaurant not called (new) | `ORDER_NOT_READY` |
| rider not recorded on `orders` | repaired first; 409 if that fails (new) | `RESTAURANT_SYNC_FAILED` |
| restaurant-service refuses | 409 with its reason | `RESTAURANT_SYNC_FAILED` |

HTTP codes are unchanged (409, 400 for cash), so older app builds, which
refresh on 400/409, behave as before. Error body now also has `error_code` and
`data: {order_id, current_status, requested_status}`.

Decision — kitchen prerequisite kept: pickup requires the restaurant to mark
the order ready (owner app, KDS or prep auto-ready timer). It is restaurant-
service's rule, it protects the kitchen workflow and inventory reconciliation,
and changing it was out of scope (restaurant-service read-only). What changed
is that the rider sees "Waiting for food to be ready" and the screen unlocks
itself, instead of a failing button. Revisit if owners forget to mark Ready.

## C. Customer-facing (new_user_app timeline)

| Step | Shown when |
|---|---|
| Order confirmed | A pending/confirmed |
| Preparing | A preparing/ready (was: ready → "Rider assigned") |
| Rider assigned | B rider_assigned/rider_arrived_restaurant, or a rider name present |
| Picked up | B picked_up (A is already out_for_delivery) |
| On the way | B out_for_delivery/on_the_way, or A out_for_delivery |
| Arriving soon | B arrived_at_customer (no backend step emits it today) |
| Delivered | A delivered/completed or B delivered |

## D. Rider app steps

| Current B | Button | Sends |
|---|---|---|
| rider_assigned | I reached restaurant | rider_arrived_restaurant |
| rider_arrived_restaurant, pickup_ready=false | Waiting for food to be ready (refreshes; auto every 15 s) | — |
| rider_arrived_restaurant | Picked up order | picked_up |
| picked_up | Start delivery | on_the_way |
| on_the_way / out_for_delivery | Mark delivered | delivered |

Active delivery response (`GET /api/v1/orders/active`) now adds
`customer_name`, `customer_phone`, `items_summary`, `restaurant_order_status`,
`pickup_ready`, `next_delivery_status` (all additive).

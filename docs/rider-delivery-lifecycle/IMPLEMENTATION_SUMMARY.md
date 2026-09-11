# Implementation summary

No schema change, no migration, no restaurant-service / restaurant-owner
change, no payment-logic change, no status renamed. All uncommitted.

## rider-service

| File | Change |
|---|---|
| `internal/client/restaurant_client.go` | trim trailing `/` from the base URL (the 404 root cause); `CallbackError` keeps restaurant-service's status and `message` |
| `internal/service/delivery_lifecycle.go` (new) | error codes; kitchen prerequisite; repair of a missing assignment before canonical steps (sync) and on reach-restaurant (best effort); acceptance-time sync with retries on 5xx/transport errors; release of deliveries the owner closed; safe log fields |
| `internal/service/delivery_service.go` | `acceptRequest` uses the new sync; `UpdateDeliveryStatus`: idempotent retry, closed-order release, coded refusals, structured events; dead `callbackRiderAssigned` removed |
| `internal/service/order_service.go` | active delivery adds customer name/phone, items, restaurant status, `pickup_ready`, `next_delivery_status`; hides an order the owner closed; event without values |
| `internal/repository/delivery_repo.go` | `GetOrderRiderSnapshot` (read), `FindDeliveriesClosedByRestaurant`, `ReleaseClosedDelivery` (guarded tx) |
| `internal/worker/closed_delivery_worker.go` (new) + `cmd/server/main.go` | sweep every 30 s, first at start |
| `internal/handler/delivery_handler.go`, `internal/dto/response.go` | `error_code` + `data.current_status` on refusals; HTTP codes unchanged |
| `internal/models/models.go` | additive `ActiveOrder` fields |
| `internal/dispatchtrace/dispatchtrace.go` | lifecycle events and reason codes |
| `internal/testpg/testpg.go` | `LifecycleSchema` (users, orders subset, history, earnings — mirrored from production) |

New log lines (`[DISPATCH] event=…`, ids and statuses only):
`restaurant.assignment.synced|sync_failed` (trigger, http_status, reason),
`delivery.status.updated` (from/to, result ok|unchanged),
`delivery.status.rejected` (current, requested, expected_status,
restaurant_status, http_status, reason_code), `rider.active_delivery.returned`
(navigation_data complete|missing_pickup|missing_drop, has_* booleans,
rider_on_order), `delivery.released`.

## rider-app

| File | Change |
|---|---|
| `lib/features/delivery/presentation/active_delivery_screen.dart` | Pickup and Drop cards (name, address, Navigate to…, Call…), current stop highlight, "Waiting for food to be ready" state with 15 s auto-refresh, specific error messages |
| `lib/core/services/map_launcher_service.dart` | `navigateTo`: Android `google.navigation:q=` then `https://www.google.com/maps/search/?api=1&query=`; address fallback, encoded; injectable launcher |
| `lib/features/delivery/services/delivery_action_policy.dart` | `isWaitingForKitchen`, `deliveryUpdateErrorMessage` |
| `lib/features/delivery/models/delivery_models.dart` | `restaurantOrderStatus`, `pickupReady`, `nextDeliveryStatus` |
| `lib/features/delivery/providers/rider_delivery_provider.dart` | backend reason in the debug-only log |

## new_user_app

| File | Change |
|---|---|
| `lib/features/tracking/domain/tracking_timeline.dart` (new) | timeline from order + delivery status + rider presence |
| `lib/features/tracking/presentation/tracking_screen.dart` | uses it; 5 s poll while a partner is being found, else 15 s; any socket event refetches |
| `lib/features/tracking/domain/tracking_refresh_policy.dart` | interval rules |
| `lib/features/orders/...` | `deliveryStatus` parsed from `/track` |

## Deployment

1. Deploy rider-service (image build; env unchanged). Optional: remove the
   trailing slash from `RESTAURANT_SERVICE_INTERNAL_BASE_URL` too.
2. Make sure `INTERNAL_SERVICE_TOKEN` is live in restaurant-service
   (restart it if the `.env` change was never deployed); otherwise every
   callback is 503 and pickups fail with `RESTAURANT_SYNC_FAILED`.
3. Ship rider-app and new_user_app builds (both are backward compatible with
   the old backend, and the backend with the old apps).
4. Run MANUAL_QA_CHECKLIST.md.

## Rollback

Redeploy the previous rider-service image: behaviour returns to today's
(callbacks 404). Data written by the new code is ordinary: rider fields on
`orders` via restaurant-service's own endpoint, and `cancelled` deliveries for
owner-closed orders. Nothing to revert. App builds can stay: new fields are
optional and old error handling still applies.

## Recommended follow-ups (not done)

- restaurant-service: publish a customer order-status event from
  `internal/orders/:id/assign-rider` so the rider appears instantly rather
  than on the 5 s poll (tiny, but restaurant-service was out of scope).
- rider-service `GET /api/v1/delivery/orders/:orderId/tracking` and
  `/ws/tracking/orders/:id` are unauthenticated and expose drop address, rider
  phone and rider location for any order id. Pre-existing; not used by the
  apps inspected. Put behind auth or remove.
- Decide the restaurant-owned `picked_up` next step (failing baseline test).

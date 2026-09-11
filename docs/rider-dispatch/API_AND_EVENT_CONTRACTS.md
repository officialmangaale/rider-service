# API and event contracts

No contract changed. This records what the rider app and rider-service actually
exchange, verified in code, so the fix builds on it rather than inventing a new one.

## Offer creation

One row in `delivery_order_requests` per rider per delivery order
(`UNIQUE (delivery_order_id, rider_id)`). `CreateRequest` inserts the row, or
reopens it if it was `rejected/expired/cancelled`. It returns no row (and no
offer is sent) if that rider's request is still `pending`. Offer lifetime:
`REQUEST_EXPIRY_SECONDS` (default 30).

Riders who **declined** are now filtered out before `CreateRequest`, so in practice
only expired or cancelled rows are reopened.

## Push: WebSocket `wss://rider-prod.mangaale.com/ws/rider?token=<JWT>`

Auth: HS256 JWT, `sub` = rider user id. Server pings every 30 s and closes after
60 s without a pong.

```json
{
  "type": "DELIVERY_ORDER_REQUEST",
  "data": {
    "request_id": 901, "order_id": 13286, "restaurant_id": 27,
    "restaurant_name": "…", "restaurant_phone": "…",
    "pickup_address": "…", "drop_address": "…",
    "pickup_latitude": 28.41, "pickup_longitude": 77.04,
    "drop_latitude": 28.41, "drop_longitude": 77.04,
    "distance_km": 0.0, "amount": 250, "payment_mode": "cash",
    "expires_at": "2026-09-11T06:40:33Z", "assignment_type": "platform"
  }
}
```

No customer name or phone is included before acceptance.

Other rider events: `ORDER_REQUEST_EXPIRED` {request_id, order_id},
`ORDER_ASSIGNED_TO_OTHER_RIDER` {request_id, order_id}, `order_assigned`
(restaurant-owned assignment).

## Poll: `GET /api/v1/riders/order-requests` (Bearer JWT)

Returns the rider's `pending`, unexpired requests. Each item comes from the same
`BuildDeliveryOrderRequestPayload` as the socket event, so the two transports
deliver identical offers. The app merges them and deduplicates by offer key.

## Offer identity (rider app)

`requestOfferKey = "<request_id>@<expires_at as whole UTC seconds>"`
(`lib/features/delivery/services/incoming_alert_policy.dart`).
- Same offer over socket and poll → same key → one ring.
- Lapsed request reopened with a new expiry → new key → rings once.
- Both sides format `expires_at` as RFC 3339 without fractions from the same
  instant, so the keys match.

## Accept / decline

- `POST /api/v1/riders/order-requests/:id/accept`: row-locked, and
  `AssignRider … WHERE assigned_rider_id IS NULL` makes it exclusive. Other
  pending requests are cancelled and those riders get `ORDER_ASSIGNED_TO_OTHER_RIDER`.
- `POST /api/v1/riders/order-requests/:id/reject`: marks only that rider's
  request `rejected`; the order is not cancelled.

## After accept → restaurant-service

`POST {RESTAURANT_SERVICE_INTERNAL_BASE_URL}/internal/orders/:id/assign-rider`
with header `X-Internal-Service-Token: $INTERNAL_SERVICE_TOKEN`, retried 3 times.
**Currently refused with 503 in production**; see
[IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md). This callback sets
`orders.assigned_rider_*`, which is what the customer app's rider card reads.

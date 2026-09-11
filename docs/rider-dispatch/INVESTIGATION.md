# Rider dispatch investigation — order 13286

> **Correction (2026-09-11, later the same day).** The first failed boundary
> for 13286 was not the stale location. `DeliveryRepository.FindNearestRiders`
> is invalid SQL (`HAVING` without `GROUP BY`), and PostgreSQL rejects it on
> every call. No offer could have been created even for a rider with a fresh
> location. The re-dispatch worker added below calls the same query, and its
> `sqlmock` tests could not detect this. The stale location and missing
> re-dispatch described here were real contributing conditions. See
> `docs/rider-offer-investigation/ROOT_CAUSE.md`.

Date: 2026-09-11. All database access was read-only (`default_transaction_read_only=on`).
Customer and rider personal data is redacted; the rider is referred to as **R1**
(user id `c6b4…30c4`).

## Symptom

A customer placed delivery order 13286. The owner confirmed it and the customer
app showed "Order confirmed" and "Finding a delivery partner". Rider R1 was
online, showed "Tracking active", and saw **"Reconnecting live orders…"**. No
request card appeared and nothing rang.

## Timeline (UTC; IST = UTC+5:30)

| Time | Event | Source |
|---|---|---|
| 05:50:20 | R1's last GPS point before dispatch | `rider_location_history.recorded_at` |
| 06:05:49.69 | Order created, `pending` | `order_status_history` |
| 06:06:03.16 | Owner confirms, `confirmed` | `order_status_history` (changed_by `owner`) |
| 06:06:03.198 | rider-service creates `delivery_orders` row 16 (`assignment_type=platform`, `restaurant_owned=false`) | `delivery_orders.created_at` |
| 06:06:03.204 | Row set to `no_rider_found`; **zero** `delivery_order_requests` rows | `delivery_orders.updated_at` |
| 06:06:18 | R1's next GPS point, 15 seconds too late | `rider_location_history` |
| 06:09:41, 06:13:11 | R1 GPS points; order still never offered | `rider_location_history` |

The trigger worked: restaurant-service published the order, and rider-service
consumed it within 40 ms. The order's `metadata.delivery_mode` is
`restaurant_own_rider`, but the restaurant had no live own rider, so it was sent
for platform dispatch exactly as designed (`restaurant-service/services/own_rider_dispatch.go`).

## Root causes

Five separate breaks. Any one of 1–4 is enough to stop a rider seeing the order;
5 stops everything after acceptance.

### 1. Rider GPS was stale at the single moment of dispatch — confirmed

`FindNearestRiders` (`rider-service/internal/repository/delivery_repo.go`)
requires `rider_locations.last_updated_at >= NOW() - 5 minutes`. At 06:06:03,
R1's last point was 15 min 43 s old, so the search returned nobody (R1 was 0.00 km
from the pickup).

The rider app stops location tracking whenever it leaves the foreground
(`rider-app/lib/features/delivery/providers/rider_delivery_provider.dart`,
`handleAppLifecycleState` → `stopTracking()`). The test used one phone for all
three apps. While the tester was in the customer and owner apps, the rider app was
in the background and sent no GPS.

### 2. An order that found no rider was never offered again — confirmed

`ProcessOrderPlacedEvent` (`internal/service/delivery_service.go`) searches once.
With no rider it sets `no_rider_found`, and nothing in rider-service revisits it.
`ExpiryWorker` only expires requests. Re-dispatch happens only if restaurant-service
re-publishes the order on a later status change. **All 16 `delivery_orders` rows
in production are `no_rider_found`**: the platform flow has never produced an offer.

### 3. The rider live-orders WebSocket cannot connect in production — confirmed

Probe through the production proxy, with correct upgrade headers and no credentials:

```
GET https://rider-prod.mangaale.com/ws/tracking/orders/0   (no auth on this route)
→ HTTP/1.1 400 Bad Request, Sec-Websocket-Version: 13, body "Bad Request"
```

That is gorilla/websocket's handshake rejection (`server.go`: "'upgrade' token not
found in 'Connection' header"). The request carried the headers, so the nginx in
front of rider-service drops them: it lacks `proxy_http_version 1.1` and the
`Upgrade`/`Connection` headers. `/ws/rider` returns 401 only because auth runs
before the upgrade. With a valid token it would hit the same 400. The rider app's
"Reconnecting live orders…" is this, permanently. The nginx config is not in any
repository; it lives only on the server.

### 4. The app's polling fallback had been removed — confirmed

Commit `f21e4c9` (2026-08-30) turned `_startFallbackPolling()` into a no-op that
only cleared a flag, assuming the socket always reconnects. With the socket
unable to connect, pending requests were fetched only at app start, go-online, or
resume. Those fetches marked requests as seen without ringing
(`refreshPendingRequests`).

### 5. rider-service → restaurant-service callbacks are refused — confirmed

```
POST https://restaurant-prod.mangaale.com/internal/orders/0/assign-rider  (no token header)
→ HTTP 503 {"message":"internal service authentication not configured"}
```

`restaurant-service/middleware/internal_auth.go` returns 503 when
`INTERNAL_SERVICE_TOKEN` is empty. So `NotifyRiderAssigned` and
`NotifyDeliveryStatusUpdate` (`rider-service/internal/client/restaurant_client.go`)
fail after 3 retries. After a rider accepts, the customer app would keep "Finding a
delivery partner", the owner would see no rider, and the order would never be
marked delivered in restaurant-service. `INTERNAL_SERVICE_TOKEN` is also absent
from both local `.env` files.

### Secondary finding — socket token frozen at construction

`riderSocketServiceProvider` passed `prefs.accessToken` as a value. `ApiClient`
refreshes tokens without rebuilding the provider, so after the token expired every
reconnect would be refused while REST kept working. Not the cause here (cause 3
blocks the socket first), but it would have been the next failure.

## Why the owner and customer flows worked

Owner confirm and the customer status update are restaurant-service only. They
use restaurant-service's DB writes and its own realtime path. Nothing there
depends on rider-service matching, the rider-prod proxy, or the internal callback.

## Other data points

- `rider_availability` shows 6 riders `is_online=true`. Five have not sent GPS
  since June, because the online flag never expires. The 5-minute GPS rule already
  excludes them correctly.
- R1: `is_online`, `is_available`, no current order.
- Exclusive accept is already correct: `AssignRider` updates only
  `WHERE assigned_rider_id IS NULL` (`internal/repository/assign_rider_race_test.go`).
- 13 old orders (June and 2026-09-09) still read `confirmed` or `preparing` in
  `orders` and have `no_rider_found` delivery rows. Any re-dispatch must not
  touch them.

See [READ_ONLY_DB_CHECKS.md](READ_ONLY_DB_CHECKS.md) for the queries.

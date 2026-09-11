# Fix plan

Smallest set of changes that restores the platform rider flow, in the order
they unblock it. Nothing rewrites dispatch, accept, or the owner/customer flows.

| # | Break | Fix | Where | Type |
|---|---|---|---|---|
| 2 | `no_rider_found` is final | `RedispatchWorker` re-offers unmatched orders every 20 s | rider-service | code |
| 1 | GPS stale at the one dispatch instant | Covered by #2: a rider who becomes eligible later gets the order within one sweep | rider-service | code |
| 4 | App polling removed | Restore polling: 10 s while the socket is down, 30 s while up; ring on new offers | rider-app | code |
| 3 | Proxy drops the WebSocket upgrade | Add a `/ws/` location with upgrade headers to the rider-prod nginx site | server | config |
| 5 | Internal callbacks refused (503) | Set one shared `INTERNAL_SERVICE_TOKEN` in rider-service and restaurant-service | server | config |
| — | Socket token frozen | Read the token on every connect | rider-app | code |

## Design decisions

**Re-dispatch runs as a sweep, not on rider location updates.** A sweep has one
code path, bounded load (≤ 50 orders per 20 s), and re-checks the current row
before offering. Triggering on every location update would put dispatch inside
the location write path.

**Guards on re-dispatch** (`redispatchCandidateSQL`):
- Only platform orders with no assigned rider, not `restaurant_owned`.
- The restaurant order must still be `accepted/confirmed/preparing/ready`. That is
  the same gate restaurant-service uses to publish, so cancelled and completed
  orders are never offered.
- Only orders first dispatched within the last **2 hours**, the same ceiling as
  restaurant-service's own-rider hold. Production has months-old orders still
  reading `confirmed`.
- Skipped while any offer is pending, and for **60 s** after the last offer
  expired, so an unanswered offer isn't re-rung straight away.
- `RedispatchOrder` re-reads the row through the existing `canRedispatch` before
  offering, to cover a rider accepting or the owner assigning between the sweep's
  SELECT and the offer.

**Riders who declined are never re-offered that order.** This applies to both the
first dispatch path and re-dispatch (`findOfferableRiders`). A rider whose offer
merely expired can be offered again.

**A re-offer rings.** `CreateRequest` reopens the same row, so the request id is
reused. The app now tracks alerts per offer (`request_id` + `expires_at`), so a
re-offer rings once and a replay of the same offer stays silent.

**No schema change.** Everything uses existing tables and columns. No migration.

## Not changed, deliberately

- **Background GPS while online.** Foreground-only tracking is why R1 was stale.
  Tracking while backgrounded needs a foreground service and the
  `ACCESS_BACKGROUND_LOCATION` Play policy declaration. That is a product and
  store-policy decision. With re-dispatch, a backgrounded rider gets the order
  within about 20–30 s of opening the app.
- **Push notifications for offers.** None exist; a backgrounded app is not woken.
  Same decision as above.
- **The 5-minute GPS freshness rule.** It correctly excludes 5 riders whose
  online flag has been stuck since June.

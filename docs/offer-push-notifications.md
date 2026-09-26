# Delivery-offer push notifications

## Why a rider with the app closed never heard about an order

Two independent faults, both confirmed.

1. **rider-service never sent a push.** `DeliveryService.sendOffers` persisted the
   offer and emitted it on the rider's WebSocket, and that was all. A phone with
   the app in the background or gone has no socket, so the only other path was the
   app's own polling, which exists only while the Online foreground service runs.
   The service had no FCM client at all (no Firebase dependency, no send call).
2. **No rider phone was ever registered for push.** The rider app posted its FCM
   token as `{"platform", "device_token"}`, while `POST /notifications/device-token`
   required `push_token`; every registration failed validation with a 400 that the
   app swallowed. Read-only check of production `notification_devices` on
   2026-09-26: **one** rider row in total, last written 2026-03-21. Separately,
   the repository's upsert named the table's primary key (a generated UUID) as its
   conflict target, so it could never match, and a repeat registration of the same
   token failed on the `(tenant, user, token)` unique index instead.

## What changed

| Area | Change |
| --- | --- |
| `internal/push/fcm.go` | FCM HTTP v1 client. Stdlib only: RS256 service-account JWT → OAuth2 token (cached, renewed, replaced once on a 401) → `messages:send`. `ErrUnregistered` for retired tokens; one retry for 429/5xx. |
| `internal/push/notifier.go` | Offer → per-device messages. Non-blocking (bounded, drops under backlog), prunes retired tokens, never logs tokens or addresses. |
| `internal/service/delivery_service.go` | `sendOffers` pushes right after each offer row is persisted and the socket message sent. `acceptRequest` tells riders whose offers were withdrawn that the offer closed. Off unless `SetOfferPusher` is called. |
| `internal/repository/notification_repo.go` | Registration fixed (UPDATE, then INSERT … DO NOTHING). A token belongs to whoever signed in last; at most 5 tokens per rider; `RemoveDeviceToken`, `DeviceTokens`. |
| `internal/dto`, `handler`, `router` | Registration accepts `device_token` **or** `push_token`; validates platform/token. New `DELETE /api/v1/notifications/device-token`. |
| `internal/config`, `cmd/server` | `FCM_SERVICE_ACCOUNT_JSON` / `FCM_SERVICE_ACCOUNT_FILE` (or `GOOGLE_APPLICATION_CREDENTIALS`), optional `FCM_PROJECT_ID`. Missing or unusable credentials log loudly and disable push; they never stop dispatch. |

No migration. The `notification_devices` table already exists (restaurant-service
migration 016); rows are written under `tenant_id = 'rider'`.

## Required configuration (not in code)

1. In the Firebase project the **rider** app belongs to, create a service account
   allowed to send (role *Firebase Cloud Messaging API Admin*), and enable the
   *Firebase Cloud Messaging API (V1)*.
2. Give rider-service `FCM_SERVICE_ACCOUNT_JSON` (raw or base64) or a key file path.
   Startup then logs `FCM push for delivery offers enabled project=<id>`. Without
   it: `FCM_SERVICE_ACCOUNT_JSON not set: delivery offers will NOT be pushed …`.
3. Deploy rider-service. **Deploy it before, or together with, the app build**: the
   registration alias means already-installed apps start registering on their next
   launch, without an update.
4. Prerequisite unrelated to push: the offer itself still needs migrations
   078/095/096/097 on the production database and the dispatch flag (see
   `restaurant-service/docs/dispatch-missing-migrations-investigation.md`). No offer
   exists without them, so no push is sent.

## Message contract (shared with the rider app)

Data-only, `android.priority = HIGH`, TTL = the offer's remaining life,
`collapse_key = offer-<request_id>`. Data values are strings.

`DELIVERY_ORDER_REQUEST`: `request_id`, `order_id`, `order_ref` (`#<order_id>`),
`order_type`, `restaurant_name`, `pickup_address`, `delivery_area`, `distance_km`
(rider → pickup), `delivery_distance_km`, `amount`, `payment_mode`, `expires_at`
(RFC 3339 UTC).

`DELIVERY_ORDER_REQUEST_CLOSED`: `order_id`, `order_type`, `reason`
(`assigned_to_other`). Sent to riders whose offers were cancelled because another
rider accepted.

Android delivery is data-only because FCM cannot attach Accept/Decline buttons to
a notification message; the app builds the notification itself in its background
message handler. iOS receives an APNs alert with category `DELIVERY_OFFER` (the
iOS app does not register that category yet, so iOS shows the alert without
buttons; see limits).

**Never in a push:** customer name, phone, coordinates or drop address. Those stay
behind acceptance, as in the offer payload. `delivery_area` is the rounded
distance ("Approx. 4 km from pickup"): the data model has no structured locality
for a delivery, and a heuristic over the free-text address could put a house
number on a lock screen. `amount` is the order value the offer already carried;
the rider's payout is a flat credit at delivery and is not part of an offer.

## Limits

- Delivery is best effort. FCM does not deliver to an app the user **Force
  Stopped**, and some OEM battery managers delay or drop background messages.
  The Online foreground service's polling and the in-app socket remain the
  fallbacks; the push is an addition.
- Only Android has Accept/Decline. iOS needs the category registered in the app
  (`DarwinNotificationCategory`) and a Mac to verify.
- Order cancelled by the restaurant/customer while offers are pending: no closing
  push is sent (no event reaches rider-service for it). The notification clears at
  the offer's expiry, the Online service's poll removes it sooner, and tapping
  Accept is refused by the backend.

## Verification performed (2026-09-26)

Go, against a scratch PostgreSQL 18 owned by the test run (never production):

- `internal/push`: JWT signature verified with the public key exactly as Google
  would, token caching/renewal, retry rules, retired-token mapping, message shape.
- `TestPreparingAnOrderPushesTheOfferToEachEligibleRiderOnce`, `…IneligibleRider…`,
  `…CannotBeDispatched…`, `…DecliningOnlyCloses…`,
  `TestTheRiderWhoLosesTheAcceptRaceIsToldTheOfferIsGone` (two riders accept
  concurrently: exactly one wins, the other gets one closure push, the winner
  none), `…WithNoPusherStillWorks`.
- Token registration: the previous statement is reproduced failing; repeat,
  refresh, hand-over between riders, five-token cap, sign-out, and other tenants'
  rows untouched. Handler test posts the app's exact request body.
- Existing `TestOnlineDispatch*` suite (including the accept/decline race) passes.
  With `TEST_DATABASE_URL` set, 20 legacy fixture tests fail exactly as they did at
  HEAD before this change; without it, the whole suite passes.

**Not verified here (needs staging and a device):** an actual FCM delivery. No
Firebase credential was available, so `FCMClient` is verified against a fake
Google, not the real service.

## Device runbook

With staging credentials configured, one rider Online, and the restaurant
allow-listed for dispatch:

1. Confirm `notification_devices` gained a `rider` row for the test rider after
   the app launched (`select count(*) … where tenant_id='rider'`).
2. App **foreground**, then **background**, then **swiped away** (not Force
   Stop), then **locked screen**: start preparation on a delivery order. Expect
   the offer popup (foreground) or a heads-up notification with Accept/Decline.
3. Tap the body → the offer sheet. Tap Accept → the notification becomes
   "Accepting…", then "Delivery accepted #…" only after the server confirms.
4. Two riders: both receive it; Accept on both at once; the loser's notification
   disappears (closure push) or reports "already taken".
5. Let one expire untouched; airplane-mode Accept; sign out and confirm the phone
   stops receiving that rider's offers.

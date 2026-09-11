# Implementation summary

Why order 13286 never reached the rider: [INVESTIGATION.md](INVESTIGATION.md).

## Code changes

### rider-service
- `internal/repository/redispatch_repo.go` (new): `FindRedispatchCandidates`,
  `DeclinedRiderIDs`.
- `internal/service/delivery_service.go`: the find-and-offer step moved out of
  `ProcessOrderPlacedEvent` into `findOfferableRiders` and `sendOffers`, with the
  same behavior. New `RedispatchOrder`. Riders who declined are excluded in both
  paths.
- `internal/worker/redispatch_worker.go` (new): sweep every
  `REDISPATCH_INTERVAL_SECONDS` (default 20; `0` disables).
- `internal/config/config.go`, `cmd/server/main.go`: config and wiring.

### rider-app
- `rider_delivery_provider.dart`: polling restored (10 s while the socket is down,
  30 s while it is up, one request in flight at a time). Polled new offers ring.
  Offers are deduplicated by offer key. On resume while online: fetch pending
  offers and reconnect the socket.
- `rider_socket_service.dart`: token read on every connect; no second socket while
  one is opening; stale-channel callbacks ignored; 15 s handshake timeout.
- `incoming_alert_policy.dart`: per-offer alert keys.
- `dashboard_screen.dart`: the banner says requests are still being checked while
  the socket is down.

No database migration. No changes to restaurant-service, new_user_app or restaurant-owner.

## Server configuration changes — required, not code

### A. rider-prod nginx: allow the WebSocket upgrade

In the rider-prod `server { … }` block (the HTTPS one), **above** `location /`,
use the same `proxy_pass` target as the existing `location /`:

```nginx
location /ws/ {
    proxy_pass http://127.0.0.1:<same port as location />;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

Then run `sudo nginx -t && sudo systemctl reload nginx`. Verify:

```bash
curl -sS -i --http1.1 -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  --max-time 4 https://rider-prod.mangaale.com/ws/tracking/orders/0 | head -1
# expect: HTTP/1.1 101 Switching Protocols   (today: 400 Bad Request)
```

### B. Shared internal service token

Generate once (`openssl rand -hex 32`) and set the **same** value as
`INTERNAL_SERVICE_TOKEN` in both the rider-service and restaurant-service
production environments. Restart both. Verify:

```bash
curl -sS -X POST -d '{}' https://restaurant-prod.mangaale.com/internal/orders/0/assign-rider
# expect: 401 "missing X-Internal-Service-Token header"   (today: 503 not configured)
```

Keep the value out of git.

## Test results

- rider-service: `go vet ./...` clean; `go test ./...` all pass; `-race -count=3`
  pass for repository, service and worker.
- rider-app: `flutter analyze` no issues; `flutter test` 62 pass, 1 pre-existing
  unrelated failure (see TEST_PLAN.md).
- Candidate SQL verified read-only on production: selects 13283 and 13286 only.

## Not verified

- On-device: ring, card, accept and reconnect after the nginx change (TEST_PLAN
  manual steps).
- The rider-app socket service has no unit test (its URL is a compile-time
  constant); covered by manual QA.
- Whether restaurant-prod's own WebSockets have the same proxy problem (they
  require auth to reach the upgrade).

## Rollback

- rider-service: set `REDISPATCH_INTERVAL_SECONDS=0` and restart. This restores
  the old "search once" behavior without a redeploy. Full revert is safe; there
  is no schema change.
- rider-app: revert the commit. Polling and socket changes are client-only.
- nginx: remove the `/ws/` block. It only adds headers for `/ws/`.

## Remaining risks

- **Backgrounded riders.** GPS pauses in the background, so a rider is ineligible
  5 minutes after leaving the app and is only reached when they reopen it.
  Fixing this needs background location and a foreground service (product and
  Play-policy decision).
- **Offer-vs-accept race.** A sweep can offer rider B moments after rider A
  accepts. B's accept is then refused ("order already assigned"). Exclusivity
  holds; B just sees a stale card.
- **Offer lifetime.** Offers last 30 s. With 10 s polling and the socket down, a
  rider gets ≥ 20 s to respond. After the nginx fix, delivery is immediate.
- **`/internal/*` is internet-reachable.** It is protected only by the token once
  set. Restricting it at nginx is a sensible follow-up.

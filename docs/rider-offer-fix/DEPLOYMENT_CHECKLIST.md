# Deployment checklist — rider-service only

No migration. No restaurant-service, owner-app or customer-app change.

## Before

- [ ] Commit rider-service. The working tree also contains the rider
      background-mode location changes (`dto/request.go`,
      `handler/location_handler.go`, `service/location_service.go`), which
      this fix builds on; ship them together.
- [ ] Optional: run the integration tests against an empty database
      (`TEST_DATABASE_URL`, `TEST_REDIS_URL`; see `internal/testpg`).
- [ ] Confirm nothing will be offered retroactively (read-only):
      the re-dispatch candidate query returns 0 rows (it did on 2026-09-11;
      re-run it right before deploying: query in `READ_ONLY_CHECKS.md`).

## Deploy

1. [ ] Build the image: `docker build -t rider-service:<tag> .`
2. [ ] Deploy / restart rider-service with the existing environment.
3. [ ] Redis reachable: logs show `[INFO] Redis dispatch cache enabled`
       (or the `[WARN] … SQL fallback` line, which is also fine).
4. [ ] Rider app online, **on screen**. A location upload lands in
       PostgreSQL (read-only):
       `SELECT now() - last_updated_at FROM rider_locations WHERE rider_id = '<id>';` → under a minute.
5. [ ] Redis received it:
       `redis-cli ZSCORE rider:locations <rider_id>` → a score;
       `redis-cli HGET rider:location_updated_at <rider_id>` → recent epoch.
6. [ ] Place one test delivery order within 5 km of the rider.
7. [ ] Owner accepts.
8. [ ] Logs:
       `docker logs <rider-service> 2>&1 | grep '\[DISPATCH\]' | tail -20` shows
       `dispatch.eligibility.evaluated … result=ok eligible_count≥1` (not `eligibility_query_failed`).
9. [ ] Offer row exists:
       `SELECT request_id, rider_id, status FROM delivery_order_requests WHERE order_id = <order>;`
10. [ ] `websocket.offer.lookup … connections=1 result=enqueued`; the rider app
        shows the request card.
11. [ ] Ringtone plays once.
12. [ ] Rider accepts → `dispatch.offer.accepted`; `delivery_orders.delivery_status = rider_assigned`.
13. [ ] A second online rider no longer sees it (`dispatch.offer.withdrawn`,
        card disappears).
14. [ ] Customer app shows the rider. Requires `INTERNAL_SERVICE_TOKEN` set
        in both rider-service and restaurant-service; otherwise
        `[RESTAURANT-CLIENT] … status 503` appears and the customer and owner
        apps do not update.
15. [ ] Fallback: `redis-cli DEL rider:locations`, place another order →
        `dispatch.search.redis … reason_code=redis_no_candidates`, offer still created.

## Verification already done (not on production)

| Command | Result |
|---|---|
| `gofmt -l internal cmd` | clean |
| `go vet ./...` | clean |
| `go test ./...` (integration tests skip) | all ok |
| `TEST_DATABASE_URL=… TEST_REDIS_URL=… go test -count=1 ./...` | all ok, 0 skipped |
| `… go test -race -count=3 -v` service, repository, ws, cache, dispatchtrace, worker | 219 PASS, 0 FAIL, 0 SKIP, 0 data races |
| old SQL from HEAD on the test PostgreSQL | fails with the production error (the new tests catch it) |
| `git diff --check` | clean |

## Rollback

- Redeploy the previous image. Dispatch returns to "no offers" (today's state).
- No data to revert. Offers created by the fixed build are ordinary rows
  and expire in 30 s.
- If only the Redis path misbehaves: unset `REDIS_URL` (SQL-only dispatch,
  fully functional) and restart.

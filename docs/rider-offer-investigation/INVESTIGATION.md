# Rider offer investigation — 2026-09-11

**Finding:** no confirmed delivery order has ever produced a rider offer,
because rider-service's eligibility query is invalid SQL and PostgreSQL
rejects it on every call. Details and evidence: ROOT_CAUSE.md.

## Gates

| Gate | Status |
|---|---|
| 1 Baseline and discovery | done (below) |
| 2 Static flow reconstruction | done: CURRENT_FLOW.md, IDENTITY_MAP.md |
| 3 Database and runtime evidence | done: READ_ONLY_QUERIES.sql, TRACE_LEDGER.md (production, read-only). Container logs not accessible |
| 4 Observability gap analysis | done: OBSERVABILITY_PLAN.md |
| 5 Additive instrumentation | done in rider-service; rider-app deferred (reason in OBSERVABILITY_PLAN.md) |
| 6 Error-handling corrections | socket write errors, auth rejections, zero-connection publishes and search failure vs no riders are now distinguishable; no outcome changed |
| 7 Automated tests | done: TEST_RESULTS.md |
| 8 Controlled reproduction | historical replay done read-only; live post-fix trace pending approval of the fix (REPRODUCTION.md §4) |
| 9 Root cause | **proven**: ROOT_CAUSE.md |

## Baseline (before this pass)

| Repo | Branch | Commit | Pre-existing uncommitted work |
|---|---|---|---|
| restaurant-owner | main | `ebdf6e9` | none |
| restaurant-service | production | `71249bd` | none |
| rider-service | main | `aba4cd1` | location endpoint changes (`internal/dto/request.go`, `internal/handler/location_handler.go`, `internal/service/location_service.go` + 3 tests), from the rider background-mode work |
| rider-app | main | `4758c80` | background-mode work (manifest, Kotlin, providers, UI) |
| new_user_app | main | `7f5ac67` | address-flow work |
| user-service | production | `c1ef001` | address-flow work |

All pre-existing work was left untouched. Baseline `go test ./...` in
rider-service: all packages pass.

Configuration (keys only): rider-service `.env` has `REDIS_URL`,
`SQS_ORDERS_QUEUE_URL` set; `SEARCH_RADIUS_KM`, `MAX_RIDERS_TO_NOTIFY`,
`REQUEST_EXPIRY_SECONDS`, `REDISPATCH_INTERVAL_SECONDS` unset → defaults
5 km / 5 / 30 s / 20 s. Rider app: REST `https://rider-prod.mangaale.com`,
socket `wss://rider-prod.mangaale.com/ws/rider` (probe → 101 after the
2026-09-11 nginx change). Database clock `now()` matched local UTC within
one second.

## Correction to an earlier report

`docs/rider-dispatch/INVESTIGATION.md` (same day) attributed order 13286 to
a stale rider location and to missing re-dispatch. Both were real, but the
first failed boundary was this query: it would have failed even with a fresh
location, and the re-dispatch worker added then calls the same query. That
report's `sqlmock` tests could not detect it. Note added there.

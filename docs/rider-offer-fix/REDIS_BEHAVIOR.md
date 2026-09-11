# Redis dispatch behaviour

**PostgreSQL is the source of truth. Redis only proposes candidates.**

## Why the old fast path could not work

`RedisDispatchCache.FindNearestRiders` required, per rider: a GEO entry, a
fresh `rider:location_updated_at` timestamp, membership of the available set,
and an availability hash saying online/available/idle.

- Location entries were written only by `POST /api/v1/riders/location`,
  which the app does not call.
- Availability was written only by `POST /api/v1/riders/availability`. The
  app goes online with `/api/v1/rider/go-online`, and accept, delivery and
  go-offline never update Redis.

So Redis always returned nobody. Populating it naively would have been
worse: a stale "available" entry could offer a busy rider.

## Now

1. Both location routes, **after** their PostgreSQL write succeeds, call
   `DeliveryService.IndexRiderLocation` → `GEOADD rider:locations` +
   `HSET rider:location_updated_at`. A Redis error is logged as
   `rider.location.index_failed` (no coordinates) and never fails the upload.
   A PostgreSQL failure fails the upload and indexes nothing.
2. Dispatch asks Redis for candidates only: `NearbyCandidateIDs` = GEO
   radius search + location timestamp < 5 min. Redis availability is not
   consulted.
3. PostgreSQL re-checks the candidates with exactly the dispatch rules
   (`FindNearestRidersAmong`, built from the same `nearestRidersSQL`).
4. The Redis result is used only if it already fills the offer list
   (`MAX_RIDERS_TO_NOTIFY`, default 5). Otherwise, or on any Redis error,
   the full SQL search runs.

Consequences:

- Stale Redis data can never produce a wrong offer (step 3).
- A rider missing from Redis is never skipped (step 4).
- At current scale (a handful of riders) the SQL search runs on almost every
  dispatch. The Redis path pays off only when an area has more fresh riders
  than the offer limit.

Each dispatch logs one line:
`event=dispatch.search.redis result=used|fallback_sql reason_code=redis_unavailable|redis_no_candidates|redis_too_few_verified candidates=N verified=M`.

## Rehydration: option A (no startup job)

Right after deploy Redis holds no rider locations. Every online rider
uploads every 15–45 s (app on screen, or the background-mode service once
shipped), so the index fills within one upload interval. Until then, and
whenever Redis is empty or down, the SQL search covers dispatch. No
startup rehydration was added: it would duplicate what the next upload does
and add a startup dependency for no correctness gain.

## Operations

- `REDIS_URL` unset or unreachable → cache disabled at startup (existing
  behaviour); dispatch is SQL-only and fully functional.
- Flushing Redis is safe at any time.

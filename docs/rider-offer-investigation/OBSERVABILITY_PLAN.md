# Observability — gaps found and what was added

## Gap inventory (before)

| Boundary | State | Detail |
|---|---|---|
| Owner accept → trigger (restaurant-service) | fully observable | structured `logger` at every guard and publish |
| SQS publish | observable | publish + `publish_failed` logs; fire-and-forget goroutine |
| Consume + delivery row | partially | `[SQS] Consumed …`, `[DELIVERY] Delivery order created` |
| **Eligibility search failure** | **misleading** | one free-text line; DB outcome `no_rider_found` identical to "nobody nearby"; no funnel on this path |
| Zero riders | partially | funnel counts, free text |
| Target rider decision | silent | no way to see which filter excluded one rider |
| Offer persist failure | partially | free text; "already pending" indistinguishable from DB error |
| Socket publish | **silent on failure** | `SendToRider` dropped the offer when the rider had no socket; caller logged `connected=` after the fact |
| Socket auth rejection | **silent** | 401 returned, nothing logged |
| Socket write failure | **silent** | `writePump` returned without a log |
| Connection lifecycle | partially | connect/disconnect text, no id, lifetime or reason |
| Metrics | absent | rider-service has no metrics stack |
| Rider app | debug-only | `[RiderSocket]`/`[RiderDelivery]` `debugPrint` inside `assert`: absent from release builds |

## Added (rider-service, additive, no decision changed)

Package `internal/dispatchtrace`: one line per event,
`[DISPATCH] event=<name> k=v …`, keys sorted, values sanitised to
`[A-Za-z0-9._-:/@]` and truncated to 160 chars (no line or field forging).
No coordinates, addresses, phones, names, tokens or payloads are passed.

| Event | Where | Key fields |
|---|---|---|
| `dispatch.attempt.started` | `ProcessOrderPlacedEvent` | order_id, delivery_order_id, event_id, trigger, delivery_mode |
| `dispatch.eligibility.evaluated` | `traceEligibility` | result=`ok`\|`error`, reason_code=`eligibility_query_failed`, error, eligible_count, radius_km; on error: `would_be_eligible`, `rejected_counts` |
| `dispatch.no_eligible_riders` | `traceEligibility` | reason_code=`no_eligible_riders`, `rejected_counts{rider_not_available,rider_location_missing,rider_location_stale,rider_outside_radius}` |
| `dispatch.eligibility.target_rider` | trace only | per-filter booleans, `first_failure`, `location_age_s`, `distance_km` (0.1 km), `selected_by_search` |
| `dispatch.offer.persisted` / `persist_failed` | `sendOffers` | request_id, rider_id, expires_in / reason_code `offer_persist_failed`\|`offer_not_reopened` |
| `websocket.offer.lookup` | `sendOffers` | connections, enqueued, result=`enqueued`\|`not_sent`\|`partial`, reason_code `no_matching_connection`\|`socket_backpressure` |
| `websocket.connection.opened` | `HandleRiderWS` | rider_id, conn_id (random), rider_connections |
| `websocket.connection.rejected` | `HandleRiderWS` | reason_code `token_missing`\|`token_invalid`(+`token_error` class)\|`missing_subject`\|`upgrade_failed` |
| `websocket.write_failed` | `writePump` | conn_id, frame, queue_depth, error |
| `websocket.connection.closed` | `writePump` | conn_id, lifetime, reason_code `peer_closed`\|`socket_write_failed`\|`server_closed` |
| `dispatch.trace.config` | startup | active trace target and expiry |

Volume: one or two lines per dispatch attempt and per socket
open/close. Re-dispatch sweeps (every 20 s) log eligibility only when a
rider is found or a trace covers the order; the worker's own error log is
unchanged. The per-rider vector runs one extra read-only query only under
an active trace.

Other: `Hub.SendToRiderCount`, `Hub.RiderConnectionCount` (new, additive);
`SendToRider` unchanged in behaviour (now copies the client slice under the
read lock, removing a data race with `removeRiderClient`).

Correlation: `event_id` (`ORDER_PLACED:<order_id>`) + `order_id` +
`request_id` + `rider_id` + `conn_id` link restaurant-service, rider-service
and the socket. No new client-visible fields were added (a top-level
`event_id` on socket frames would change the app's de-duplication).

## Diagnostic mode (per-target trace)

Off by default. Environment of the rider-service container:

```
DISPATCH_TRACE_UNTIL=2026-09-12T12:00:00Z   # required, RFC 3339, max 24 h ahead (clamped)
DISPATCH_TRACE_RIDER_ID=<rider user id>      # required
DISPATCH_TRACE_ORDER_ID=<order id>           # optional; omit to trace every order for that rider
```

- **Enable:** add the variables, restart rider-service (env is read at start;
  the platform has no runtime flag store).
- **Verify:** `docker logs <rider-service> 2>&1 | grep 'event=dispatch.trace.config'`.
- **Disable:** remove the variables and restart, or let `DISPATCH_TRACE_UNTIL`
  pass; tracing stops automatically at that time without a restart.

No HTTP endpoint exposes or changes it.

## Not added in this pass

- **Metrics** (`dispatch_*_total` etc.): no metrics stack exists; adding one
  is a separate decision. The event names above are the counters' basis.
- **Rider-app diagnostic events:** the first failed boundary is server-side
  and no offer has ever reached the app, so the app path cannot be
  exercised until the query is fixed. Existing debug logs cover connect,
  failure, event type, duplicate and alert decisions. Recommended next:
  a bounded, redacted ring buffer of `rider_socket.*` / `rider_offer.*`
  events visible in staging builds.
- **restaurant-service:** already structured at every dispatch-trigger branch.

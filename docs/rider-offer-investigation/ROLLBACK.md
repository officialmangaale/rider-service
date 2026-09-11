# Rollback

## Instrumentation (this pass)

Additive logging only; no schema, API, status or rule change.

- Disable the target trace: remove `DISPATCH_TRACE_*` from the rider-service
  environment and restart, or let `DISPATCH_TRACE_UNTIL` pass.
- Remove the instrumentation entirely: revert the commit containing
  `internal/dispatchtrace/`, `internal/repository/rider_decision_repo.go`
  and the edits to `internal/service/delivery_service.go`,
  `internal/ws/hub.go`, `cmd/server/main.go`. No data to clean up.

Files belonging to this pass (rider-service):

```
cmd/server/main.go                          (SetTrace wiring)
internal/dispatchtrace/dispatchtrace.go     (new)
internal/dispatchtrace/dispatchtrace_test.go(new)
internal/repository/rider_decision_repo.go  (new, read-only query)
internal/service/delivery_service.go        (events; logEligibilitySummary → traceEligibility)
internal/service/dispatch_observability_test.go (new)
internal/ws/hub.go                          (conn id, events, SendToRiderCount)
internal/ws/hub_observability_test.go       (new)
docs/rider-offer-investigation/*            (new)
```

The other uncommitted rider-service files (`internal/dto/request.go`,
`internal/handler/location_handler.go`, `internal/service/location_service.go`
and their tests) belong to the rider background-mode work and are
independent of this pass.

## Proposed fix (when approved)

Single-query change in `FindNearestRiders`. Rollback: redeploy the previous
rider-service image. Effect of rolling back: dispatch returns to creating
no offers (today's behaviour). No data migration either way.

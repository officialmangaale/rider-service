# Test results — 2026-09-11

All commands in `rider-service/`.

| Command | Result |
|---|---|
| `gofmt -l internal cmd` | no output (clean) |
| `go vet ./...` | clean |
| `go test ./...` | all packages `ok` |
| `go test -race -count=2 ./internal/ws ./internal/dispatchtrace ./internal/service` | `ok` |
| `git diff --check` | clean |

## New tests

| Test | Proves |
|---|---|
| `dispatchtrace.TestEmitWritesOneStableSearchableLine` | exact, sorted, single-line format |
| `dispatchtrace.TestEmitCannotBeInjected` | a value cannot add a line or field |
| `dispatchtrace.TestLongValuesAreTruncated` | bounded line length |
| `dispatchtrace.TestTraceIsOffByDefault` | off without config, without expiry, when expired, when unparseable |
| `dispatchtrace.TestTraceTargetsAndExpires` | only the target order; stops at expiry |
| `dispatchtrace.TestTraceLeftOnByMistakeIsClamped` | ≤ 24 h |
| `dispatchtrace.TestRiderDecisionNamesTheFirstFailedFilter` | deterministic reason; eligible only when all filters pass |
| `service.TestAFailedSearchIsLoggedDifferentlyFromNoRiders` | the 13294 case: `result=error reason_code=eligibility_query_failed would_be_eligible=1` |
| `service.TestNoRidersReportsWhichFilterRemovedThem` | `rejected_counts` per filter |
| `service.TestTheTargetRiderVectorNeedsAnActiveTrace` | no per-rider vector in normal traffic; full vector under trace |
| `service.TestDispatchEventsNeverContainCoordinates` | no coordinate fragments in any line |
| `ws.TestSendToRiderCountReportsNoConnection` | zero-connection publish is observable |
| `ws.TestSendToRiderCountReportsBackpressure` | full buffer is observable |
| `ws.TestRejectedSocketsLogAReasonAndNeverTheToken` | reasons logged; no token, `eyJ…` or `token=` in logs |

## SQL verified on real PostgreSQL (read-only)

| Query | Result |
|---|---|
| `FindNearestRiders` (existing) | **ERROR** `column "rl.rider_id" must appear in the GROUP BY clause…` |
| proposed corrected shape (replay at 07:26:22.087) | returns the target rider, 0.00 km |
| `riderDecisionSQL` (new, trace) | valid; returned the live vector |
| `GetRiderEligibilitySummary` (existing, now also on the error path) | valid |

## Unchanged behaviour

Existing suites pass unchanged, including dispatch re-dispatch
(`redispatch_test.go`), assignment race (`assign_rider_race_test.go`),
own-rider gate and liveness, available orders, SQS consumer, handler and
location tests. No status value, filter, radius, freshness window, expiry
or assignment rule was modified (diff of `delivery_service.go` adds only
`dispatchtrace` calls and replaces `logEligibilitySummary` with
`traceEligibility`, which runs the same summary query on the same path).

## Baseline failures

None in rider-service. (Unrelated repos not run in this pass.)

## Not run

- Rider-app tests: rider-app unchanged in this pass.
- Device / staging end-to-end: pending the approved fix (REPRODUCTION.md §4).

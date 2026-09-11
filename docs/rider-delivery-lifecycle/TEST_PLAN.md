# Test plan and results

## rider-service (Go)

Integration tests run on real PostgreSQL + Redis when `TEST_DATABASE_URL` /
`TEST_REDIS_URL` point at empty throwaway instances (they skip otherwise).
The fake restaurant-service in `delivery_lifecycle_postgres_test.go` enforces
restaurant-service's real contract and is reached through a trailing-slash
base URL, as production was configured.

| Test | Covers |
|---|---|
| `TestTrailingSlashBaseURLStillHitsTheInternalRoutes` | `/`, `//`, spaces → never `//internal` |
| `TestRefusalKeepsRestaurantServiceReason` | status + message kept, one line |
| `TestAcceptedRiderIsRecordedOnTheCustomerOrder` | accept → rider on `orders`; other offer cancelled; second accept refused |
| `TestAcceptanceSyncRetriesAServerError` | two 500s then success |
| `TestDeliveryLifecycleValidSequence` | arrived → (kitchen ready) → picked_up → on_the_way → delivered; cash required; rider freed; 4 history rows |
| `TestPickupBeforeKitchenReadyIsRefusedClearly` | `ORDER_NOT_READY`, restaurant not called, then succeeds once ready |
| `TestStatusUpdateRecordsAMissingAssignmentFirst` | order 13356's state: repaired on reach-restaurant and on pickup |
| `TestStatusUpdateFailsClosedWhenTheAssignmentCannotBeRecorded` | `RESTAURANT_SYNC_FAILED`, no advance, reason logged |
| `TestRestaurantRefusalCarriesItsReason` | 409 + message in error and log |
| `TestRepeatingTheCurrentStepIsIdempotent` | same step twice → ok, one history row |
| `TestSkippingAStepIsRejected` | `INVALID_TRANSITION`, message unchanged, expected step logged |
| `TestOnlyTheAssignedRiderCanUpdate` | `NOT_ASSIGNED_RIDER` |
| `TestActiveDeliveryCarriesNavigationContactsAndNextStep` | both stops, contacts, kitchen state, next step; log leaks no values |
| `TestOrderClosedByTheRestaurantReleasesTheRider/{completed,cancelled,rejected}` | delivery cancelled, rider offerable again, no payout, idempotent |
| `TestStatusUpdateOnAClosedOrderReleasesAndSaysSo` | `ORDER_CLOSED`, released at once |
| `TestActiveOrderHidesAnOrderTheRestaurantClosed` | not shown to the app |
| `TestOwnerMarkedDeliveredStillLetsTheRiderFinish` | owner `delivered` is not a closure |
| `TestStatusRefusalCarriesAnErrorCodeAndTheCurrentStatus` (handler) | `error_code`, `data.current_status`, HTTP 409 |
| `TestClosedDelivery*` (worker) | sweep on start, stop twice, errors |

Mutation check: with the trailing-slash trim removed, the client test and the
lifecycle tests fail with exactly the production symptoms (404 on
assign-rider; "canonical order transition failed … 404" on pickup).

Results (2026-09-11):

| Command | Result |
|---|---|
| `gofmt -l internal cmd` | only `handler/delivery_handler.go`, `handler/upload_handler.go` — unformatted at HEAD, left as is |
| `go vet ./...` | ok |
| `go test -count=1 -v ./...` with test DBs | 119 PASS, 0 FAIL, 0 SKIP |
| `go test -race -count=5 -v` service, repository, ws, cache, dispatchtrace, worker, client, handler | 540 PASS, 0 FAIL, 0 data races |
| `go test ./...` without test DBs | all ok (integration tests skip) |
| `git diff --check` | clean |

## rider-app (Flutter) — `test/features/delivery/active_delivery_lifecycle_test.dart`

Parsing (stops, contacts, kitchen state; older backend never blocks), next
action per status, waiting-for-kitchen, current stop, error messages per
`error_code`, navigation URIs (Android intent first, website fallback, address
encoding, none when nothing to navigate to), launcher fallback order and
exceptions, stop card widget (navigates, disabled with no destination, friendly
failure). The widget tests caught a real overflow of side-by-side buttons at
phone width; buttons are now stacked.

| Command | Result |
|---|---|
| `flutter analyze` | No issues found |
| `flutter test` | 131 passed, 1 failed — `delivery_action_policy_test.dart: restaurant-owned picked-up order can only be delivered`, **failing at baseline before any change** (test expects "Mark delivered", code returns "On the way"; both came in commit 21aff3b; backend allows both). Left for a product decision. |

## new_user_app (Flutter) — `test/features/tracking/delivery_lifecycle_tracking_test.dart`

Timeline rules (ready ≠ rider assigned; rider accept moves it; picked_up vs
out_for_delivery; delivered from either field), 5 s/15 s refresh policy,
`delivery_status` parsed from the track response, tracking screen widget: rider
replaces "Finding a delivery partner", ready-without-rider stays Preparing,
pickup shows "Picked up".

| Command | Result |
|---|---|
| `flutter analyze` | 17 issues, identical to baseline (14 info, 3 warning, none in changed files) |
| `flutter test` | 167 passed (158 before + 9 new) |

Android SDK/device: not available here. See MANUAL_QA_CHECKLIST.md.

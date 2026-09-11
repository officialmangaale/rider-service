# Test plan

## Automated (added with this fix)

rider-service, `go test ./...` (all packages pass, and `-race -count=3` on the three below):

| Test | Proves |
|---|---|
| `repository/redispatch_repo_test.go` | Candidate SQL keeps every guard: assignment, restaurant-owned, restaurant order status, soft delete, 2 h ceiling, live/recent offer, and passes age/cooldown/limit |
| `TestDeclinedRiderIDsReadsOnlyRejectedRequests` | Only an explicit decline excludes a rider |
| `service/redispatch_test.go` `…OffersTheOrderToARiderWhoIsNowEligible` | The 13286 case: an unmatched order is offered once a rider is eligible |
| `…WithNoEligibleRiderWritesNothing` | A sweep that finds nobody does not write |
| `…SkipsARiderWhoDeclined` | A decliner is skipped and one extra rider is searched |
| `…StopsWhenTheOrderWasAssignedMeanwhile` | Platform accept, owner's own rider, or cancelled → no offer |
| `…StopsWhileAnOfferIsPending` | No second offer on top of a live one |
| `worker/redispatch_worker_test.go` | One failing order doesn't stop others; query failure is survived; config defaults; Start/Stop idempotent |

rider-app, `flutter test`:

| Test | Proves |
|---|---|
| `incoming_alert_policy_test.dart` | Offer key is stable across time zones and fractions; a replay is silent; a re-offer rings; a polled batch rings once |

Pre-existing, unrelated failure: `delivery_action_policy_test.dart`
("restaurant-owned picked-up order can only be delivered"). It fails identically
on the untouched HEAD.

## Manual QA, after deploy and both config changes

Use two phones, or one phone with the rider app kept in the **foreground**.

1. Rider online, "Tracking active". After the nginx change the "Reconnecting
   live orders…" banner must disappear within a few seconds.
2. Customer places a delivery order; owner confirms.
3. Rider sees the request card and hears one ring within about 10 s.
4. Rider accepts. The customer app replaces "Finding a delivery partner" with the
   rider card, and the owner app shows the rider. **Requires `INTERNAL_SERVICE_TOKEN`.**
5. Second rider online at the same time: their card disappears on accept.
6. Decline path: rider declines → order not cancelled, and not re-offered to that rider.
7. Stale-GPS path (the 13286 case): put the rider app in the background for over
   5 minutes, confirm an order, then reopen the rider app. The card arrives within
   about 30 s.
8. Ignore an offer: it disappears at expiry and returns about 60–90 s later with one ring.

Watch rider-service logs for `[REDISPATCH-WORKER] Sweep candidates=… offers=…`
and `[DELIVERY] Redispatched order … to … rider(s)`.

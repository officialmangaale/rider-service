# Online delivery branch

Branch: `feat/online-delivery-flow-20260924`, cut from local `main` at `b078b2c`.

This keeps the existing delivery/offer system and adds authoritative food-source
gating, atomic order/rider claims, idempotent acceptance, private offer previews,
durable shared-order assignment, guarded lifecycle completion, withdrawal,
reconciliation and participant-only tracking access.

Deploy only after restaurant-service migration **097_online_delivery_dispatch.sql**
has been applied to the shared database. Its flag starts OFF. The rollout,
configuration, rollback, compatibility and validation details are in the
restaurant-service `docs/online-delivery-flow.md` delivered with this change.

Local checks:

```text
go test ./...
TEST_DATABASE_URL=<empty isolated PostgreSQL> go test ./internal/service -run '^TestOnlineDispatch' -count=1
```

The PostgreSQL test fixture reads the migration from the sibling
`restaurant-service` checkout; check out the matching branch beside this repo.
Never point these tests at an application database. The test helper rejects a
database that already has public orders/users tables and creates isolated schemas.

Existing search radius, request expiry, redispatch limits and payout amounts are
retained. No new fee is introduced. The request preview's `amount` is the customer
order total, not a payout. Private drop coordinates are omitted, and
`delivery_distance_km` is a rounded straight-line estimate.

New endpoint: `POST /api/v1/riders/orders/:orderId/withdraw`, JSON `{"reason":"..."}`,
assigned rider only, food orders before pickup. Historical assignments remain.
Legacy `/orders/assignments/:id/accept` resolves to an existing current offer;
it cannot create an independent assignment. HTTP and WebSocket food tracking
require an authenticated order participant. Unsupported grocery tracking via
these private endpoints is refused; customer restaurant-service tracking remains.

The feature flag does not gate completion of assigned deliveries. For rollback,
turn it off and retain the new backend until deliveries drain. Old backend
binaries do not honor this switch. No production deploy was performed.

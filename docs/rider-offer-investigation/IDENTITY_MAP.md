# Identity map

One rider identifier is used everywhere dispatch looks: the user-service
user id (UUID), carried as the JWT `sub`.

| Where | Column / key | Type | Value for the target rider |
|---|---|---|---|
| user-service / auth | `users.id`, JWT `sub` | `uuid` | `c6b46748-16e5-443a-8daf-c82ffe6130c4` |
| Availability | `rider_availability.rider_id` | `varchar(255)` | same, as text |
| Current location | `rider_locations.rider_id` | `varchar(255)` | same |
| Location history | `rider_location_history.rider_id` | `uuid` | same |
| Restaurant-owned link | `restaurant_riders.rider_user_id` | `varchar` | none (self-signup) |
| Offer | `delivery_order_requests.rider_id` | `varchar(255)` | written from `rider_locations.rider_id` |
| Assignment | `delivery_orders.assigned_rider_id`, `rider_user_id` | `varchar` | — |
| Order (restaurant side) | `orders.assigned_rider_user_id` | `varchar(100)` | — |
| Socket hub | `Hub.riderClients[sub]`, channel `rider:<sub>` | string | same |
| Redis | `rider:availability:<id>`, GEO member `<id>` | string | not populated by the app's upload route |
| REST auth | `middleware.GetUserID` (JWT `sub`) → location writes | string | same |

Verified: all rows for the target rider carry the same text value (Q5);
there is no separate "rider profile id". The `varchar = uuid` join trap
exists only where `users.id` is joined; `HasActiveRestaurantOwnRiders`
already casts (`u.id::text`).

Order identifiers: `orders.order_id` (int) = `delivery_orders.order_id` =
`delivery_order_requests.order_id`; `delivery_orders.delivery_order_id` is
rider-service's own key. Correlation across services: `event_id =
ORDER_PLACED:<order_id>` (restaurant publisher → SQS → rider-service
`processed_events`, and now every `[DISPATCH]` line).

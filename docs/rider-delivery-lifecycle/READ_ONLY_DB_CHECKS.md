# Read-only production checks (2026-09-11)

All with `PGOPTIONS='-c default_transaction_read_only=on'` (verified `on`).
No writes. Rider ids truncated; no phones, addresses or coordinates recorded.

| Check | Result |
|---|---|
| Delivery orders, last 6 h | 13356 `rider_arrived_restaurant` (rider c6b46748…), 13360 and 13374 `no_rider_found` |
| Offers | one ever accepted: request 1, order 13356, accepted 12:15:55 |
| 13356 delivery timeline | assigned 12:15:55, arrived 12:16:16, no pickup |
| 13356 `orders` row | order_status then `ready` (12:17:33), delivery_status `pending`, **no rider in any rider column** |
| Delivery orders with a rider on `orders` (60 days) | **0** |
| Column types used by assign-rider UPDATE | compatible (rider_id bigint receives NULL for UUIDs; others varchar) |
| 13356 order history | confirmed 12:15:48 → preparing 12:17:21 → ready 12:17:33 → **completed by owner 12:50:05** |
| Rider c6b46748… availability | online, available, **current_order_id = 13356** (excluded from dispatch) |
| Offers for 13360, 13374 | 0 (the only fresh rider was busy with 13356) |
| NULLs in delivery_orders restaurant_name/phone, assignment_type, restaurant_owned | 0 of 21 |
| Release sweep dry run (query below) | exactly 1 row: 13356 / c6b46748… / rider_arrived_restaurant / completed |
| Active deliveries overall | 1 (13356) |

Queries (order ids as psql variables):

```sql
SELECT d.order_id, d.delivery_status, left(d.assigned_rider_id, 8) rider,
       d.assigned_at, d.rider_arrived_at, d.picked_up_at, d.delivered_at,
       o.order_status, o.delivery_status AS orders_delivery_status,
       (COALESCE(o.assigned_rider_user_id, '') <> '') AS rider_on_orders
FROM delivery_orders d JOIN orders o USING (order_id)
WHERE d.order_id = :order_id;

SELECT previous_status, status, changed_by, source, changed_at
FROM order_status_history WHERE order_id = :order_id ORDER BY changed_at;

SELECT left(rider_id, 8), is_online, is_available, current_order_id
FROM rider_availability WHERE rider_id = :'rider_id';

-- What the first ClosedDeliveryWorker sweep after deploy will release:
SELECT d.delivery_order_id, d.order_id, d.delivery_status, LOWER(TRIM(o.order_status))
FROM delivery_orders d JOIN orders o ON o.order_id = d.order_id
WHERE d.is_deleted = FALSE
  AND d.delivery_status IN ('rider_assigned','rider_arrived_restaurant','picked_up','on_the_way')
  AND LOWER(TRIM(COALESCE(o.order_status,''))) IN ('completed','cancelled','rejected');
```

Not done (blocked by policy, correctly): an unauthenticated probe of
`https://restaurant-prod.mangaale.com//internal/...`. The 404 was reproduced
locally with the same gin version instead. Confirm in production with:

```
docker logs <rider-service> 2>&1 | grep RESTAURANT-CLIENT | tail
```

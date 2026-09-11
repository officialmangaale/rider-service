# Manual QA checklist — delivery lifecycle

Prerequisites
- [ ] rider-service deployed with this change; `INTERNAL_SERVICE_TOKEN` set to
      the same value in rider-service **and** restaurant-service (restaurant-
      service restarted so it is live).
- [ ] Logs show `[CLOSED-DELIVERY-WORKER] Started interval=30s` and, on the
      first sweep, `event=delivery.released order_id=13356 … restaurant_status=completed`
      (frees rider c6b46748… for new offers).
- [ ] New rider-app and new_user_app builds installed. Rider app on screen,
      Online, location fresh.

Flow (one test delivery order within 5 km, cash payment)
1. [ ] Place a delivery order from new_user_app.
2. [ ] Owner accepts in the owner app (unchanged flow).
3. [ ] Rider receives the real-time offer (ringtone once).
4. [ ] Rider accepts. Log: `dispatch.offer.accepted`, then
       `restaurant.assignment.synced … trigger=accept` (not `sync_failed`).
5. [ ] new_user_app within ~5 s: "Finding a delivery partner" replaced by the
       rider's name; timeline "Rider assigned". Read-only check:
       `SELECT assigned_rider_user_id IS NOT NULL FROM orders WHERE order_id = …` → true.
6. [ ] Rider sees the active delivery: Pickup card (restaurant, address,
       Navigate to restaurant, Call restaurant) and Drop card (customer name,
       address, Navigate to customer, Call customer). Pickup marked "Current stop".
7. [ ] Tap Navigate to restaurant.
8. [ ] Google Maps opens turn-by-turn to the pickup. With Google Maps
       uninstalled/disabled: the browser opens google.com/maps instead.
9. [ ] Tap "I reached restaurant" → status "Arrived at restaurant".
10. [ ] Customer app unchanged or "Rider assigned" (no customer step for this).
11. [ ] While the order is Confirmed/Preparing: button reads "Waiting for food
        to be ready" with an explanation; tapping it only refreshes. Owner marks
        Ready → within 15 s the button becomes "Picked up order". Tap it.
        Log: `delivery.status.updated … to_status=picked_up result=ok`.
12. [ ] Customer app shows "Picked up" immediately (order-status socket).
13. [ ] Drop card becomes "Current stop"; tap Navigate to customer.
14. [ ] Google Maps opens the drop route.
15. [ ] Tap "Start delivery" → customer "On the way".
16. [ ] (No separate "Arrived at customer" step exists in the rider flow; the
        customer's "Arriving soon" step is not driven by the backend today.)
17. [ ] Tap "Mark delivered" → cash confirmation dialog → Collected.
18. [ ] Customer app shows "Delivered"; socket and polling stop.
19. [ ] Rider is free: back on home, `rider_availability.current_order_id` NULL,
        receives the next offer.
20. [ ] A second online rider never sees the order after step 4
        (`dispatch.offer.withdrawn`).

Negative checks
- [ ] Double-tap a step / retry after airplane mode → no error, no duplicate.
- [ ] Owner marks the order Completed or Cancelled mid-delivery → within 30 s
      (or on the rider's next tap: "The restaurant closed this order…") the
      rider's active delivery disappears and they get offers again.
- [ ] Stop with no saved location → Navigate disabled with "No location saved".
- [ ] Restaurant-owned rider flow (owner assigns own rider): Confirm pickup →
      On the way → Mark delivered still works.
- [ ] Owner app, KDS, POS: create/accept/prepare/ready unchanged.

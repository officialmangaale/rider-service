package service

import (
	"context"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// Phase 6: a grocery order is dispatched by the same machinery as a food one.
// These tests exist to prove two things that could otherwise go wrong quietly:
// a grocery order id never collides with a food order id, and a grocery
// delivery reports to the grocery side of restaurant-service, not the food one.

func groceryOrderEvent(groceryOrderID int) *models.OrderPlacedEvent {
	return &models.OrderPlacedEvent{
		EventType:       "ORDER_PLACED",
		EventID:         "ORDER_PLACED:grocery:" + itoa(groceryOrderID),
		SourceOrderType: models.SourceOrderTypeGrocery,
		OrderID:         groceryOrderID,
		MerchantID:      77,
		MerchantName:    "Anita Daily Needs",
		MerchantPhone:   "+919000000077",
		// A grocery event carries the shop in the restaurant_* fields too, so
		// one dispatch path reads either kind.
		RestaurantID:    77,
		RestaurantName:  "Anita Daily Needs",
		RestaurantPhone: "+919000000077",
		OrderType:       "DELIVERY",
		DeliveryMode:    "platform",
		PaymentMode:     "cod",
		Amount:          310,
		ItemsSummary:    "Toor Dal x2",
		Pickup:          models.LocationDetail{Latitude: e2ePickupLat, Longitude: e2ePickupLng, Address: "shop"},
		Drop:            models.LocationDetail{Latitude: e2ePickupLat + 2*e2eKm, Longitude: e2ePickupLng, Address: "drop"},
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func TestGroceryOrderIsOfferedLikeAFoodOrder(t *testing.T) {
	l := newLifecycle(t)
	e2eSeedRider(t, l.db, lcRider, 0.5)
	ctx := context.Background()

	if err := l.svc.ProcessOrderPlacedEvent(ctx, groceryOrderEvent(42)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	var orderType, status, name string
	var amount float64
	if err := l.db.QueryRow(`SELECT order_type, delivery_status, restaurant_name, amount
		FROM delivery_orders WHERE order_id = 42 AND order_type = 'grocery'`).
		Scan(&orderType, &status, &name, &amount); err != nil {
		t.Fatalf("grocery delivery order: %v", err)
	}
	if orderType != models.SourceOrderTypeGrocery || name != "Anita Daily Needs" || amount != 310 {
		t.Fatalf("delivery order = %s %q %.2f", orderType, name, amount)
	}

	// The nearby rider was offered it, exactly as for food.
	requestID, requestStatus := requestFor(t, l.db, 42, lcRider)
	if requestStatus != models.RequestStatusPending {
		t.Fatalf("offer status = %q, want pending", requestStatus)
	}
	if requestID <= 0 {
		t.Fatal("no offer was created for the nearby rider")
	}
}

func TestGroceryAndFoodOrderIDsDoNotCollide(t *testing.T) {
	l := newLifecycle(t)
	e2eSeedRider(t, l.db, lcRider, 0.5)
	ctx := context.Background()

	// The same numeric id, one food order and one grocery order. Before the
	// order type was part of the key this was impossible: the second one
	// silently updated the first.
	if _, err := l.db.Exec(`INSERT INTO orders (order_id, restaurant_id, order_status) VALUES (777, 27, 'confirmed')`); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := l.svc.ProcessOrderPlacedEvent(ctx, confirmedOrderEvent(777)); err != nil {
		t.Fatalf("food dispatch: %v", err)
	}
	if err := l.svc.ProcessOrderPlacedEvent(ctx, groceryOrderEvent(777)); err != nil {
		t.Fatalf("grocery dispatch: %v", err)
	}

	var rows int
	if err := l.db.QueryRow(`SELECT count(*) FROM delivery_orders WHERE order_id = 777`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("delivery orders for id 777 = %d, want 2 (one per type)", rows)
	}
	var foodName, groceryName string
	if err := l.db.QueryRow(`SELECT restaurant_name FROM delivery_orders WHERE order_id = 777 AND order_type = 'food'`).
		Scan(&foodName); err != nil {
		t.Fatal(err)
	}
	if err := l.db.QueryRow(`SELECT restaurant_name FROM delivery_orders WHERE order_id = 777 AND order_type = 'grocery'`).
		Scan(&groceryName); err != nil {
		t.Fatal(err)
	}
	if foodName != "Test Kitchen" || groceryName != "Anita Daily Needs" {
		t.Fatalf("the two deliveries were merged: food=%q grocery=%q", foodName, groceryName)
	}
}

func TestGroceryDispatchIsIdempotent(t *testing.T) {
	l := newLifecycle(t)
	e2eSeedRider(t, l.db, lcRider, 0.5)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := l.svc.ProcessOrderPlacedEvent(ctx, groceryOrderEvent(43)); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
	}
	var deliveries, offers int
	if err := l.db.QueryRow(`SELECT count(*) FROM delivery_orders WHERE order_id = 43 AND order_type = 'grocery'`).
		Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if err := l.db.QueryRow(`SELECT count(*) FROM delivery_order_requests r
		JOIN delivery_orders d ON d.delivery_order_id = r.delivery_order_id
		WHERE d.order_id = 43 AND d.order_type = 'grocery'`).Scan(&offers); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 {
		t.Fatalf("delivery orders = %d, want 1", deliveries)
	}
	if offers != 1 {
		t.Fatalf("offers to the one nearby rider = %d, want 1", offers)
	}
}

func TestGroceryOfferTellsTheRiderItIsGrocery(t *testing.T) {
	l := newLifecycle(t)
	e2eSeedRider(t, l.db, lcRider, 0.5)
	ctx := context.Background()
	if err := l.svc.ProcessOrderPlacedEvent(ctx, groceryOrderEvent(44)); err != nil {
		t.Fatal(err)
	}

	order, err := l.svc.deliveryRepo.GetDeliveryOrderByOrderID(ctx, 44, models.SourceOrderTypeGrocery)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := requestFor(t, l.db, 44, lcRider)
	payload := BuildDeliveryOrderRequestPayload(
		&models.DeliveryOrderRequest{RequestID: request, OrderID: 44},
		order, 1.2, order.CreatedAt)

	if payload["order_type"] != models.SourceOrderTypeGrocery {
		t.Fatalf("offer order_type = %v, want grocery", payload["order_type"])
	}
	if payload["merchant_name"] != "Anita Daily Needs" || payload["items_summary"] != "Toor Dal x2" {
		t.Fatalf("offer = %v", payload)
	}
	if payload["payment_mode"] != "cod" || payload["amount"] != float64(310) {
		t.Fatalf("offer money = %v / %v", payload["payment_mode"], payload["amount"])
	}
	// The customer's name and number are not in the offer, only in the order
	// the rider reads after accepting.
	for _, key := range []string{"customer_name", "customer_phone", "drop_landmark"} {
		if _, present := payload[key]; present {
			t.Fatalf("offer exposes %q before acceptance", key)
		}
	}
}

func TestFoodOfferStillSaysFood(t *testing.T) {
	l := newLifecycle(t)
	e2eSeedRider(t, l.db, lcRider, 0.5)
	ctx := context.Background()
	if _, err := l.db.Exec(`INSERT INTO orders (order_id, restaurant_id, order_status) VALUES (888, 27, 'confirmed')`); err != nil {
		t.Fatal(err)
	}
	if err := l.svc.ProcessOrderPlacedEvent(ctx, confirmedOrderEvent(888)); err != nil {
		t.Fatal(err)
	}
	order, err := l.svc.deliveryRepo.GetDeliveryOrderByOrderID(ctx, 888, "")
	if err != nil {
		t.Fatalf("an empty order type must mean food: %v", err)
	}
	payload := BuildDeliveryOrderRequestPayload(
		&models.DeliveryOrderRequest{RequestID: 1, OrderID: 888}, order, 1.0, order.CreatedAt)
	if payload["order_type"] != models.SourceOrderTypeFood {
		t.Fatalf("food offer order_type = %v, want food", payload["order_type"])
	}
	if payload["restaurant_name"] != "Test Kitchen" {
		t.Fatalf("food offer lost its restaurant: %v", payload["restaurant_name"])
	}
}

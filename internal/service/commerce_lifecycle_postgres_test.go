package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

func TestCommerceGroceryActiveAndCompletionDoNotReadFoodCollision(t *testing.T) {
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatal(err)
	}
	const rider = "c6b46748-0000-4000-8000-000000000031"
	if _, err := db.Exec(`INSERT INTO users(id) VALUES($1);`, rider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO orders(order_id,restaurant_id,order_status) VALUES(7000,1,'delivered');
	CREATE TABLE grocery_pricing_snapshots(grocery_order_id BIGINT PRIMARY KEY,rider_cost_cents BIGINT);
	INSERT INTO grocery_pricing_snapshots VALUES(7000,4550);`); err != nil {
		t.Fatal(err)
	}
	var deliveryID int
	err := db.QueryRow(`INSERT INTO delivery_orders(order_id,order_type,restaurant_id,customer_id,
	pickup_latitude,pickup_longitude,pickup_address,drop_latitude,drop_longitude,drop_address,
	amount,payment_mode,delivery_status,assigned_rider_id,rider_user_id)
	VALUES(7000,'grocery',11,0,28.4,77.0,'Grocery shop',28.5,77.1,'Grocery customer',250,'cod','on_the_way',$1,$1) RETURNING delivery_order_id`, rider).Scan(&deliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO rider_availability(rider_id,is_online,is_available,current_order_id) VALUES($1,true,false,7000)`, rider); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE delivery_orders SET restaurant_name='Grocery shop',restaurant_phone='',customer_name='Customer',customer_phone='',items_summary='Item x1' WHERE delivery_order_id=$1`, deliveryID); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewDeliveryRepository(db)
	svc := NewOrderService(nil, repo, nil, nil, nil, nil, nil)
	active, err := svc.GetActiveOrder(context.Background(), rider)
	if err != nil || active.OrderType != "grocery" || active.DropAddress != "Grocery customer" {
		t.Fatalf("grocery hidden by food collision: %+v %v", active, err)
	}
	var callbacks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/grocery/orders/7000/delivery-status" || r.Header.Get("X-Internal-Service-Token") != "commerce-test" {
			t.Errorf("incorrect canonical callback %s", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		var payload client.GroceryDeliveryStatusPayload
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload.RiderID != rider || payload.DeliveryStatus != "delivered" {
			t.Error("incorrect grocery status payload")
			w.WriteHeader(400)
			return
		}
		callbacks.Add(1)
		// Remove the unrelated food row at the canonical callback boundary.
		// Grocery history and payout must still commit with no matching food.
		if _, err := db.Exec(`DELETE FROM orders WHERE order_id=7000`); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	deliverySvc := &DeliveryService{deliveryRepo: repo, riderRepo: repository.NewRiderRepository(db), hub: ws.NewHub(), restaurantCli: client.NewRestaurantClient(server.URL, "commerce-test")}
	for retry := 0; retry < 2; retry++ {
		if err := deliverySvc.UpdateDeliveryStatus(context.Background(), 7000, rider, models.DeliveryStatusDelivered, true, "", "grocery"); err != nil {
			t.Fatalf("complete retry=%d err=%v", retry, err)
		}
	}
	if callbacks.Load() != 1 {
		t.Fatalf("duplicate/wrong canonical callbacks: %d", callbacks.Load())
	}
	var history, earnings int
	var balance float64
	if err = db.QueryRow(`SELECT count(*) FROM delivery_status_history WHERE order_type='grocery' AND order_id=7000`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM rider_earnings WHERE order_type='grocery' AND order_id=7000`).Scan(&earnings); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT balance FROM rider_wallet_balances WHERE rider_id=$1`, rider).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if history != 1 || earnings != 1 || balance != -204.5 {
		t.Fatalf("history=%d earnings=%d net wallet=%v", history, earnings, balance)
	}
}

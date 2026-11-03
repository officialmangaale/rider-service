package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"testing"
	"time"
)

func TestRestaurantAssignmentUsesFoodKeyAndPreservesPickup(t *testing.T) {
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN business_phone text;
 ALTER TABLE orders ADD COLUMN customer_id bigint;
 INSERT INTO restaurants(restaurant_id,name,street_address) VALUES(19,'Iconic','Pickup');
 INSERT INTO orders(order_id,restaurant_id,order_status,assigned_rider_user_id) VALUES(81,19,'preparing','hired');
 INSERT INTO delivery_orders(order_id,order_type,restaurant_id,customer_id,pickup_latitude,pickup_longitude,drop_latitude,drop_longitude,delivery_status)
 VALUES(81,'grocery',19,0,0,0,0,0,'pending');`); err != nil {
		t.Fatal(err)
	}
	repo := NewDeliveryRepository(db)
	event := &models.RiderAssignedToOrderEvent{OrderID: 81, RestaurantID: 19, RiderUserID: "hired", AssignedAt: time.Now().UTC().Format(time.RFC3339)}
	order, err := repo.UpsertRestaurantOwnedOrder(context.Background(), event)
	if err != nil || order == nil || order.OrderType != "food" || !order.RestaurantOwned {
		t.Fatalf("assignment failed: %+v %v", order, err)
	}
	if _, err := db.Exec(`UPDATE delivery_orders SET delivery_status='picked_up',picked_up_at=now() WHERE order_id=81 AND order_type='food'`); err != nil {
		t.Fatal(err)
	}
	order, err = repo.UpsertRestaurantOwnedOrder(context.Background(), event)
	if err != nil || order.DeliveryStatus != "picked_up" || order.PickedUpAt == nil {
		t.Fatalf("late assignment rewound pickup: %+v %v", order, err)
	}
	var groceryStatus string
	if err := db.QueryRow(`SELECT delivery_status FROM delivery_orders WHERE order_id=81 AND order_type='grocery'`).Scan(&groceryStatus); err != nil || groceryStatus != "pending" {
		t.Fatalf("grocery overlap changed: %s %v", groceryStatus, err)
	}
	event.RiderUserID = "other"
	if _, err := repo.UpsertRestaurantOwnedOrder(context.Background(), event); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale rider assignment accepted: %v", err)
	}
}

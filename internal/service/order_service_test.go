package service

import (
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

func TestActiveOrderFromDeliveryOrderIncludesNavigationAndPhoneFields(t *testing.T) {
	assignedAt := time.Now()
	order := &models.DeliveryOrder{
		DeliveryOrderID: 10,
		OrderID:         1958,
		RestaurantID:    27,
		RestaurantName:  "Mangaale Kitchen",
		RestaurantPhone: "911234567890",
		PickupLatitude:  28.6139,
		PickupLongitude: 77.2090,
		PickupAddress:   "Pickup address",
		DropLatitude:    28.6200,
		DropLongitude:   77.2100,
		DropAddress:     "Drop address",
		Amount:          500,
		PaymentMode:     "cod",
		DeliveryStatus:  models.DeliveryStatusRiderAssigned,
		AssignmentType:  "platform",
		AssignedAt:      &assignedAt,
	}

	active := activeOrderFromDeliveryOrder(order)

	if active.RestaurantPhone != order.RestaurantPhone {
		t.Fatalf("restaurant_phone mismatch: got %q", active.RestaurantPhone)
	}
	if active.PickupLatitude == nil || *active.PickupLatitude != order.PickupLatitude {
		t.Fatalf("pickup_latitude missing or incorrect")
	}
	if active.DropLongitude == nil || *active.DropLongitude != order.DropLongitude {
		t.Fatalf("drop_longitude missing or incorrect")
	}
	if active.DeliveryStatus != models.DeliveryStatusRiderAssigned {
		t.Fatalf("delivery_status mismatch: got %q", active.DeliveryStatus)
	}
}

func TestActiveOrderFromSharedOrderIncludesCustomerAndRestaurantPhoneWhenAvailable(t *testing.T) {
	deliveryAddress := "Customer address"
	restaurantLat := 18.5204
	restaurantLng := 73.8567
	dropLat := 18.5300
	dropLng := 73.8600
	order := &models.Order{
		OrderID:           42,
		RestaurantID:      7,
		OrderStatus:       "ready",
		TotalAmount:       350,
		DeliveryAddress:   &deliveryAddress,
		DeliveryLatitude:  &dropLat,
		DeliveryLongitude: &dropLng,
		RestaurantName:    "Shared Restaurant",
		RestaurantAddress: "Restaurant address",
		RestaurantLat:     &restaurantLat,
		RestaurantLng:     &restaurantLng,
		RestaurantPhone:   "02012345678",
		CustomerPhone:     "9876543210",
	}

	active := activeOrderFromSharedOrder(order)

	if active.CustomerPhone != "9876543210" {
		t.Fatalf("customer_phone missing: got %q", active.CustomerPhone)
	}
	if active.RestaurantPhone != "02012345678" {
		t.Fatalf("restaurant_phone missing: got %q", active.RestaurantPhone)
	}
	if active.PickupLatitude == nil || *active.PickupLatitude != restaurantLat {
		t.Fatalf("pickup coordinates missing")
	}
	if active.DropLatitude == nil || *active.DropLatitude != dropLat {
		t.Fatalf("drop coordinates missing")
	}
}

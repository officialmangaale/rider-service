package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

func TestBuildDeliveryOrderRequestPayloadIncludesRestaurantName(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Second)
	req := &models.DeliveryOrderRequest{
		RequestID:       99,
		DeliveryOrderID: 12,
		OrderID:         1958,
		DistanceKm:      1.75,
		ExpiresAt:       expiresAt,
	}
	order := &models.DeliveryOrder{
		DeliveryOrderID: 12,
		OrderID:         1958,
		RestaurantID:    27,
		RestaurantName:  "Mangaale Kitchen",
		PickupAddress:   "Pickup address",
		DropAddress:     "Drop address",
		PickupLatitude:  28.6139,
		PickupLongitude: 77.2090,
		DropLatitude:    28.6200,
		DropLongitude:   77.2100,
		Amount:          500,
	}

	payload := BuildDeliveryOrderRequestPayload(req, order, req.DistanceKm, expiresAt)

	if payload["restaurant_name"] != "Mangaale Kitchen" {
		t.Fatalf("restaurant_name missing from request payload: %#v", payload)
	}
	if payload["request_id"] != 99 || payload["order_id"] != 1958 {
		t.Fatalf("request/order ids not preserved: %#v", payload)
	}
}

func TestDeliveryProofRequestAcceptsPaymentCollectedAndNotes(t *testing.T) {
	var req dto.DeliveryProofRequest
	body := []byte(`{"payment_collected":true,"notes":"Cash collected and order delivered"}`)
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("delivery proof payload should unmarshal: %v", err)
	}
	if req.PaymentCollected == nil || !*req.PaymentCollected {
		t.Fatalf("payment_collected not parsed")
	}
	if req.Notes != "Cash collected and order delivered" {
		t.Fatalf("notes not parsed: %q", req.Notes)
	}
}

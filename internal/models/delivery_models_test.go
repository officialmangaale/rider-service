package models

import (
	"encoding/json"
	"testing"
)

func TestOrderPlacedEventAcceptsTextCustomerID(t *testing.T) {
	raw := []byte(`{
		"event_type": "ORDER_PLACED",
		"order_id": 1195,
		"restaurant_id": 21,
		"customer_id": "customer-uuid",
		"order_type": "DELIVERY",
		"delivery_mode": "platform",
		"pickup": {"latitude": 12.9716, "longitude": 77.5946, "address": "MG Road"},
		"drop": {"latitude": 12.9352, "longitude": 77.6245, "address": "Indiranagar"}
	}`)

	var evt OrderPlacedEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatalf("unmarshal text customer_id: %v", err)
	}
	if evt.CustomerID.Int() != 0 {
		t.Fatalf("expected non-numeric customer_id to map to 0, got %d", evt.CustomerID.Int())
	}
	if evt.DeliveryMode != "platform" {
		t.Fatalf("expected delivery mode to survive unmarshal, got %q", evt.DeliveryMode)
	}
}

func TestOrderPlacedEventAcceptsNumericCustomerID(t *testing.T) {
	raw := []byte(`{"customer_id": 42}`)

	var evt OrderPlacedEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatalf("unmarshal numeric customer_id: %v", err)
	}
	if evt.CustomerID.Int() != 42 {
		t.Fatalf("expected numeric customer_id 42, got %d", evt.CustomerID.Int())
	}
}

package service

import (
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

func TestCanonicalizeOrderPlacedEventUsesStableOrderID(t *testing.T) {
	evt := &models.OrderPlacedEvent{
		EventType: " order_placed ",
		EventID:   "8a1d9f83-fc00-411f-9a3d-6f77f908a020",
		OrderID:   42,
	}

	if err := canonicalizeOrderPlacedEvent(evt); err != nil {
		t.Fatalf("canonicalize event: %v", err)
	}

	if evt.EventType != "ORDER_PLACED" {
		t.Fatalf("expected normalized event type, got %q", evt.EventType)
	}
	if evt.EventID != "ORDER_PLACED:42" {
		t.Fatalf("expected stable order event ID, got %q", evt.EventID)
	}
}

func TestCanonicalizeOrderPlacedEventDefaultsMissingTypeAndID(t *testing.T) {
	evt := &models.OrderPlacedEvent{OrderID: 77}

	if err := canonicalizeOrderPlacedEvent(evt); err != nil {
		t.Fatalf("canonicalize event: %v", err)
	}

	if evt.EventType != "ORDER_PLACED" || evt.EventID != "ORDER_PLACED:77" {
		t.Fatalf("unexpected canonical event: %+v", evt)
	}
}

func TestCanonicalizeOrderPlacedEventRejectsNil(t *testing.T) {
	if err := canonicalizeOrderPlacedEvent(nil); err == nil {
		t.Fatal("expected nil event error")
	}
}

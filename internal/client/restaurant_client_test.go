package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyDeliveryStatusUpdateUsesAuthenticatedCanonicalEndpoint(t *testing.T) {
	var received DeliveryStatusPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if r.URL.Path != "/internal/orders/42/delivery-status" {
			t.Errorf("path = %s; want canonical delivery-status endpoint", r.URL.Path)
		}
		if got := r.Header.Get("X-Internal-Service-Token"); got != "shared-secret" {
			t.Errorf("internal token = %q; want shared-secret", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRestaurantClient(server.URL, "shared-secret")
	err := client.NotifyDeliveryStatusUpdate(42, DeliveryStatusPayload{
		OrderID:          42,
		RestaurantID:     7,
		RiderID:          "rider-1",
		DeliveryStatus:   "picked_up",
		PaymentCollected: false,
	})
	if err != nil {
		t.Fatalf("NotifyDeliveryStatusUpdate returned error: %v", err)
	}
	if received.OrderID != 42 || received.RestaurantID != 7 || received.RiderID != "rider-1" {
		t.Fatalf("unexpected payload: %+v", received)
	}
	if received.DeliveryStatus != "picked_up" {
		t.Fatalf("delivery status = %q; want picked_up", received.DeliveryStatus)
	}
}

func TestNotifyDeliveryStatusUpdateFailsClosedWithoutRestaurantService(t *testing.T) {
	client := NewRestaurantClient("", "shared-secret")
	err := client.NotifyDeliveryStatusUpdate(42, DeliveryStatusPayload{
		OrderID:        42,
		RestaurantID:   7,
		RiderID:        "rider-1",
		DeliveryStatus: "delivered",
	})
	if err == nil {
		t.Fatal("expected missing restaurant-service configuration to fail closed")
	}
}

func TestNotifyRiderAssignedFailsClosedWithoutRestaurantService(t *testing.T) {
	client := NewRestaurantClient("", "shared-secret")
	err := client.NotifyRiderAssigned(42, AssignRiderPayload{
		RiderID:   "rider-1",
		RiderName: "Rider One",
	})
	if err == nil {
		t.Fatal("expected missing restaurant-service configuration to fail closed")
	}
}

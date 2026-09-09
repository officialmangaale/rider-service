package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotifyRiderDeliveryCompletedPostsToTheReferralEndpoint(t *testing.T) {
	var received RiderDeliveryCompletedPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if r.URL.Path != "/internal/referrals/rider-delivery-completed" {
			t.Errorf("path = %s; want the referral qualification endpoint", r.URL.Path)
		}
		// The endpoint sits behind RequireInternalServiceToken; without this
		// header every callback would be rejected.
		if got := r.Header.Get("X-Internal-Service-Token"); got != "shared-secret" {
			t.Errorf("internal token = %q; want shared-secret", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewRestaurantClient(server.URL, "shared-secret")
	err := client.NotifyRiderDeliveryCompleted(RiderDeliveryCompletedPayload{
		RiderID:             "rider-user-1",
		DeliveryRef:         "order:2868",
		CompletedDeliveries: 5,
	})
	if err != nil {
		t.Fatalf("NotifyRiderDeliveryCompleted returned error: %v", err)
	}

	// The field names must match the handler's binding exactly, or the
	// referral silently never qualifies.
	if received.RiderID != "rider-user-1" {
		t.Errorf("rider_id = %q; want rider-user-1", received.RiderID)
	}
	if received.DeliveryRef != "order:2868" {
		t.Errorf("delivery_ref = %q; want order:2868", received.DeliveryRef)
	}
	if received.CompletedDeliveries != 5 {
		t.Errorf("completed_deliveries = %d; want 5", received.CompletedDeliveries)
	}
}

// The payload must serialise to the snake_case keys the handler binds to.
func TestRiderDeliveryCompletedPayloadUsesTheHandlersFieldNames(t *testing.T) {
	body, err := json.Marshal(RiderDeliveryCompletedPayload{
		RiderID: "r1", DeliveryRef: "order:1", CompletedDeliveries: 2,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"rider_id", "delivery_ref", "completed_deliveries"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("payload is missing %q; the handler binds on this name", key)
		}
	}
}

func TestNotifyRiderDeliveryCompletedReportsAnUnconfiguredService(t *testing.T) {
	client := NewRestaurantClient("", "shared-secret")
	if err := client.NotifyRiderDeliveryCompleted(RiderDeliveryCompletedPayload{
		RiderID: "rider-1",
	}); err == nil {
		t.Fatal("expected an error when no restaurant-service base URL is configured")
	}
}

// The async form must never block or panic, whatever the far side does — the
// delivery it follows has already been committed.
func TestAsyncReferralCallbackIsSafeWhenTheServiceIsUnconfigured(t *testing.T) {
	client := NewRestaurantClient("", "shared-secret")
	done := make(chan struct{})
	go func() {
		client.NotifyRiderDeliveryCompletedAsync(RiderDeliveryCompletedPayload{RiderID: "r1"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the async callback blocked with no base URL configured")
	}
}

func TestAsyncReferralCallbackRetriesAServerError(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewRestaurantClient(server.URL, "shared-secret")
	client.NotifyRiderDeliveryCompletedAsync(RiderDeliveryCompletedPayload{
		RiderID: "r1", DeliveryRef: "order:1", CompletedDeliveries: 1,
	})

	deadline := time.After(8 * time.Second)
	for atomic.LoadInt32(&calls) < 2 {
		select {
		case <-deadline:
			t.Fatalf("expected a retry; saw %d call(s)", atomic.LoadInt32(&calls))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

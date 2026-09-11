package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Production configured RESTAURANT_SERVICE_INTERNAL_BASE_URL with a trailing
// slash. restaurant-service's router (gin, default settings; nginx passes the
// path through unchanged) answers "//internal/..." with 404, so every
// assign-rider and delivery-status callback failed.
func TestTrailingSlashBaseURLStillHitsTheInternalRoutes(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "//") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	for _, base := range []string{server.URL + "/", server.URL + "//", " " + server.URL + "/ "} {
		c := NewRestaurantClient(base, "t")
		if err := c.NotifyRiderAssigned(13356, AssignRiderPayload{RiderID: "r"}); err != nil {
			t.Fatalf("base %q: assign-rider: %v", base, err)
		}
		if err := c.NotifyDeliveryStatusUpdate(13356, DeliveryStatusPayload{DeliveryStatus: "picked_up"}); err != nil {
			t.Fatalf("base %q: delivery-status: %v", base, err)
		}
	}
	for _, p := range paths {
		if strings.HasPrefix(p, "//") {
			t.Fatalf("request went to %q", p)
		}
	}
}

func TestRefusalKeepsRestaurantServiceReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":"error","message":"rider is not assigned\nto this order"}`))
	}))
	defer server.Close()

	err := NewRestaurantClient(server.URL, "t").NotifyDeliveryStatusUpdate(42, DeliveryStatusPayload{})
	var cbErr *CallbackError
	if !errors.As(err, &cbErr) {
		t.Fatalf("got %T %v, want *CallbackError", err, err)
	}
	if cbErr.StatusCode != http.StatusForbidden || cbErr.Message != "rider is not assigned to this order" {
		t.Fatalf("got %+v", cbErr)
	}
	if !strings.HasPrefix(err.Error(), "restaurant-service returned status 403 for order 42") {
		t.Fatalf("error text changed: %q", err.Error())
	}
}

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/maps"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

type stubRouteService struct {
	route  *service.DeliveryRoute
	err    error
	riders []string
	orders []int
	origin maps.Coordinate
}

func (s *stubRouteService) GetDeliveryRoute(_ context.Context, riderID string, orderID int, _ string, origin maps.Coordinate) (*service.DeliveryRoute, error) {
	s.riders = append(s.riders, riderID)
	s.orders = append(s.orders, orderID)
	s.origin = origin
	return s.route, s.err
}

func (s *stubRouteService) GetDeliveryRouteAt(ctx context.Context, riderID string, orderID int, orderType string, origin maps.Coordinate, _ time.Time) (*service.DeliveryRoute, error) {
	return s.GetDeliveryRoute(ctx, riderID, orderID, orderType, origin)
}

func routeRequest(t *testing.T, stub *stubRouteService, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &DeliveryHandler{routeSvc: stub}
	r := gin.New()
	r.POST("/api/v1/riders/orders/:orderId/route", func(c *gin.Context) {
		// The auth middleware's job: the rider identity comes from the token,
		// never from the request body.
		c.Set("user_id", "rider-from-token")
		h.GetDeliveryRoute(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/riders/orders/42/route", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRouteEndpointContractKeepsExistingFieldsAndAddsTimestamps(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	stub := &stubRouteService{route: &service.DeliveryRoute{
		EncodedPolyline: "abc", DistanceMeters: 800, DurationSeconds: 130, StaticDurationSeconds: 130,
		Provider: "google", DestinationType: "drop",
		GeneratedAt: now.Add(-3 * time.Minute), ExpiresAt: now.Add(12 * time.Minute),
		RouteDistanceMeters: 2000, RouteDurationSeconds: 400,
		ETACalculatedAt: now, RiderLocationAt: now.Add(-5 * time.Second),
		ETABasis: "route_progress", RefreshReason: "phase_change", Reused: true,
	}}
	rec := routeRequest(t, stub, `{"latitude":30.71,"longitude":76.7,"order_type":"food"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data struct {
			Route map[string]any `json:"route"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	route := envelope.Data.Route
	// The fields the Rider app has always parsed.
	for _, key := range []string{
		"encoded_polyline", "distance_meters", "duration_seconds", "static_duration_seconds",
		"provider", "destination_type", "generated_at", "expires_at",
	} {
		if _, ok := route[key]; !ok {
			t.Fatalf("route response lost existing field %q: %v", key, route)
		}
	}
	// Additive: route, ETA and rider-fix times stay distinct.
	for _, key := range []string{
		"route_distance_meters", "route_duration_seconds", "eta_calculated_at",
		"rider_location_at", "eta_basis", "refresh_reason", "reused",
	} {
		if _, ok := route[key]; !ok {
			t.Fatalf("route response missing %q: %v", key, route)
		}
	}
	if route["generated_at"] == route["eta_calculated_at"] {
		t.Fatal("route calculation time and ETA calculation time must be distinct values")
	}
	if strings.Contains(rec.Body.String(), "AIza") {
		t.Fatal("a credential-shaped string must never appear in a route response")
	}
	if len(stub.riders) != 1 || stub.riders[0] != "rider-from-token" || stub.orders[0] != 42 {
		t.Fatalf("handler must use the authenticated rider and path order: %+v %+v", stub.riders, stub.orders)
	}
}

func TestRouteEndpointErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"budget reached", &service.DeliveryRouteError{Code: service.RouteErrBudgetExceeded}, http.StatusTooManyRequests, service.RouteErrBudgetExceeded},
		{"rate limited", &service.DeliveryRouteError{Code: service.RouteErrRateLimited}, http.StatusTooManyRequests, service.RouteErrRateLimited},
		{"order not found", &service.DeliveryRouteError{Code: service.RouteErrOrderNotFound}, http.StatusNotFound, service.RouteErrOrderNotFound},
		{"bad coordinates", &service.DeliveryRouteError{Code: service.RouteErrInvalidCoordinates}, http.StatusBadRequest, service.RouteErrInvalidCoordinates},
		{"provider disabled", &service.DeliveryRouteError{Code: service.RouteErrProviderDisabled}, http.StatusServiceUnavailable, service.RouteErrProviderDisabled},
		{"upstream", &service.DeliveryRouteError{Code: service.RouteErrUpstream}, http.StatusServiceUnavailable, service.RouteErrUpstream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := routeRequest(t, &stubRouteService{err: tc.err}, `{"latitude":30.71,"longitude":76.7}`)
			if rec.Code != tc.status {
				t.Fatalf("status got %d want %d (%s)", rec.Code, tc.status, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.code) {
				t.Fatalf("response should carry error code %s: %s", tc.code, rec.Body.String())
			}
		})
	}
}

func TestRouteEndpointRejectsMalformedBodyWithoutCallingTheService(t *testing.T) {
	stub := &stubRouteService{}
	rec := routeRequest(t, stub, `not json`)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", rec.Code)
	}
	if len(stub.riders) != 0 {
		t.Fatal("a malformed request must not reach the route service")
	}
}

func TestMapsUsageEndpointReturnsCountersOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	maps.DefaultUsage.Inc("routes.reused")
	h := &DeliveryHandler{}
	r := gin.New()
	r.GET("/internal/maps/usage", h.GetMapsUsage)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/maps/usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out struct {
		Data struct {
			Counters map[string]int64 `json:"counters"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Counters["routes.reused"] < 1 {
		t.Fatalf("counter missing: %v", out.Data.Counters)
	}
}

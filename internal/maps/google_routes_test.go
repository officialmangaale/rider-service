package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/config"
)

const routeJSON = `{"routes":[{"distanceMeters":1200,"duration":"150.5s","staticDuration":"120s","polyline":{"encodedPolyline":"test-polyline"},"legs":[{"distanceMeters":1200,"duration":"150.5s","staticDuration":"120s"}]}]}`

func testConfig(url string) config.GoogleMaps {
	return config.GoogleMaps{RoutesEnabled: true, ServerAPIKey: "test-only-key", BaseURL: url, RequestTimeout: time.Second}
}
func testRequest() RouteRequest {
	return RouteRequest{Origin: Coordinate{28.61, 77.20}, Destination: Coordinate{28.62, 77.22}}
}
func quiet(context.Context, Observation) {}
func requireCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	var actual *RouteError
	if !errors.As(err, &actual) || actual.Code != want {
		t.Fatalf("got %v want %s", err, want)
	}
}

func TestRouteSuccessAndRequestContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/directions/v2:computeRoutes" || r.URL.RawQuery != "" {
			t.Errorf("bad route request")
		}
		if r.Header.Get("X-Goog-Api-Key") != "test-only-key" || strings.Contains(r.Header.Get("X-Goog-FieldMask"), "*") || !strings.Contains(r.Header.Get("X-Goog-FieldMask"), "routes.polyline.encodedPolyline") {
			t.Errorf("bad headers")
		}
		var body struct {
			Origin struct {
				Location struct {
					LatLng Coordinate `json:"latLng"`
				} `json:"location"`
			} `json:"origin"`
			TravelMode string `json:"travelMode"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Origin.Location.LatLng != testRequest().Origin || body.TravelMode != "DRIVE" {
			t.Errorf("bad body")
		}
		fmt.Fprint(w, routeJSON)
	}))
	defer server.Close()
	var observed Observation
	client := NewGoogleRoutes(testConfig(server.URL), nil, nil, func(_ context.Context, o Observation) { observed = o })
	req := testRequest()
	req.Options = RouteOptions{RoutingPreference: "TRAFFIC_AWARE", IncludePolyline: true}
	result, err := client.GetRoute(WithCorrelationID(context.Background(), "req-123"), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.DistanceMeters != 1200 || result.DurationSeconds != 150.5 || result.StaticDurationSeconds != 120 || len(result.Legs) != 1 || result.Polyline != "test-polyline" || result.TrafficDurationSeconds == nil || result.CalculatedAt.IsZero() {
		t.Fatalf("bad result: %+v", result)
	}
	if observed.CorrelationID != "req-123" || observed.Attempts != 1 || observed.StatusCode != 200 || observed.ErrorCode != "" {
		t.Fatalf("bad observation: %+v", observed)
	}
}

func TestRouteFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   ErrorCode
	}{
		{"malformed", 200, `bad-json`, InvalidResponse}, {"negative", 200, strings.Replace(routeJSON, "1200", "-1", 1), InvalidResponse},
		{"duration", 200, strings.Replace(routeJSON, "150.5s", "NaNs", 1), InvalidResponse},
		{"no legs", 200, `{"routes":[{"distanceMeters":1,"duration":"1s","staticDuration":"1s"}]}`, InvalidResponse},
		{"empty", 200, `{"routes":[]}`, NoRoute}, {"bad request", 400, `secret upstream message`, InvalidRequest},
		{"unauthorized", 401, `secret`, NotConfigured}, {"forbidden", 403, `secret`, NotConfigured},
		{"rate limit", 429, `secret`, RateLimited}, {"server", 500, `secret`, UpstreamError},
		{"too large", 200, strings.Repeat("x", maxResponseBytes+1), InvalidResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); fmt.Fprint(w, test.body) }))
			defer server.Close()
			_, err := NewGoogleRoutes(testConfig(server.URL), nil, nil, quiet).GetRoute(context.Background(), testRequest())
			requireCode(t, err, test.want)
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("upstream body leaked")
			}
		})
	}
}

func TestProviderGuardsNeverCallHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, test := range []struct {
		enabled bool
		key     string
		code    ErrorCode
	}{{false, "", ProviderDisabled}, {false, "test", ProviderDisabled}, {true, "", NotConfigured}} {
		cfg := testConfig(server.URL)
		cfg.RoutesEnabled = test.enabled
		cfg.ServerAPIKey = test.key
		client := NewGoogleRoutes(cfg, nil, nil, quiet)
		_, err := client.GetRoute(context.Background(), testRequest())
		requireCode(t, err, test.code)
		_, err = client.GetRouteMatrix(context.Background(), RouteMatrixRequest{})
		requireCode(t, err, test.code)
	}
	client := NewGoogleRoutes(testConfig(server.URL), nil, nil, quiet)
	for _, invalid := range []Coordinate{{0, 0}, {91, 1}, {1, 181}, {math.NaN(), 1}, {1, math.Inf(1)}} {
		r := testRequest()
		r.Origin = invalid
		_, err := client.GetRoute(context.Background(), r)
		requireCode(t, err, InvalidCoordinates)
		_, err = client.GetRouteMatrix(context.Background(), RouteMatrixRequest{Origins: []Coordinate{invalid}, Destinations: []Coordinate{r.Destination}})
		requireCode(t, err, InvalidCoordinates)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetRoute(ctx, testRequest())
	requireCode(t, err, Cancelled)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("lost context identity")
	}
	_, err = client.GetRouteMatrix(ctx, RouteMatrixRequest{Origins: []Coordinate{testRequest().Origin}, Destinations: []Coordinate{testRequest().Destination}})
	requireCode(t, err, Cancelled)
	if calls.Load() != 0 {
		t.Fatal("guard made network request")
	}
	for _, valid := range []Coordinate{{0, 77}, {28, 0}, {-90, 180}} {
		if !valid.Valid() {
			t.Fatal("valid Phase 1 coordinate rejected")
		}
	}
}

func TestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	cfg := testConfig(server.URL)
	cfg.RequestTimeout = 20 * time.Millisecond
	client := NewGoogleRoutes(cfg, nil, nil, quiet)
	_, err := client.GetRoute(context.Background(), testRequest())
	requireCode(t, err, Timeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lost deadline identity")
	}
}

func TestRetryPolicy(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					w.WriteHeader(status)
				} else {
					fmt.Fprint(w, routeJSON)
				}
			}))
			defer server.Close()
			cfg := testConfig(server.URL)
			cfg.MaxRetries = 1
			_, err := NewGoogleRoutes(cfg, nil, nil, quiet).GetRoute(context.Background(), testRequest())
			if status == 400 || status == 401 || status == 403 {
				if calls != 1 || err == nil {
					t.Fatal("retried permanent failure")
				}
			} else if calls != 2 || err != nil {
				t.Fatalf("transient retry failed: %v calls=%d", err, calls)
			}
		})
	}
}

func TestRetryAfterAndRedirectIsolation(t *testing.T) {
	for _, status := range []int{302, 429} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Retry-After", "30")
			w.Header().Set("Location", "https://example.invalid")
			w.WriteHeader(status)
		}))
		cfg := testConfig(server.URL)
		cfg.MaxRetries = 2
		_, err := NewGoogleRoutes(cfg, nil, nil, quiet).GetRoute(context.Background(), testRequest())
		server.Close()
		if calls != 1 || err == nil {
			t.Fatal("redirect/long Retry-After was followed")
		}
	}
}

func TestRouteMatrix(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       ErrorCode
	}{
		{"success", `[{"originIndex":0,"destinationIndex":0,"status":{},"condition":"ROUTE_EXISTS","distanceMeters":100,"duration":"30s","staticDuration":"20s"}]`, ""},
		{"no route", `[{"status":{},"condition":"ROUTE_NOT_FOUND"}]`, ""},
		{"partial error", `[{"status":{"code":8}}]`, ""},
		{"missing status", `[{"condition":"ROUTE_EXISTS"}]`, InvalidResponse},
		{"bad index", `[{"originIndex":3,"status":{},"condition":"ROUTE_NOT_FOUND"}]`, InvalidResponse},
		{"empty", `[]`, InvalidResponse}, {"invalid", `{}`, InvalidResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/distanceMatrix/v2:computeRouteMatrix" || !strings.Contains(r.Header.Get("X-Goog-FieldMask"), "status") {
					t.Error("bad matrix request")
				}
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			r := testRequest()
			result, err := NewGoogleRoutes(testConfig(server.URL), nil, nil, quiet).GetRouteMatrix(context.Background(), RouteMatrixRequest{Origins: []Coordinate{r.Origin}, Destinations: []Coordinate{r.Destination}})
			if test.want != "" {
				requireCode(t, err, test.want)
				return
			}
			if err != nil || len(result.Elements) != 1 {
				t.Fatalf("bad matrix: %+v %v", result, err)
			}
			switch test.name {
			case "success":
				if result.Elements[0].Route == nil || result.Elements[0].Route.DistanceMeters != 100 {
					t.Fatal("lost route")
				}
			case "no route":
				requireCode(t, result.Elements[0].Error, NoRoute)
			case "partial error":
				requireCode(t, result.Elements[0].Error, RateLimited)
			}
		})
	}
}

func TestDisabledAddressProviders(t *testing.T) {
	provider := DisabledAddressProvider{}
	_, err := provider.SearchPlaces(context.Background(), PlaceSearch{})
	requireCode(t, err, ProviderDisabled)
	_, err = provider.GetPlaceDetails(context.Background(), "id", "session")
	requireCode(t, err, ProviderDisabled)
	_, err = provider.ForwardGeocode(context.Background(), "address")
	requireCode(t, err, ProviderDisabled)
	_, err = provider.ReverseGeocode(context.Background(), Coordinate{})
	requireCode(t, err, ProviderDisabled)
}

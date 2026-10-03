package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/maps"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

type fakeActiveOrderResolver struct {
	order *models.ActiveOrder
	err   error
}

func (f fakeActiveOrderResolver) GetActiveOrder(context.Context, string) (*models.ActiveOrder, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.order, nil
}

type fakeRouteProvider struct {
	mu       sync.Mutex
	calls    int
	requests []maps.RouteRequest
	wait     chan struct{}
	err      error
}

func (f *fakeRouteProvider) GetRoute(_ context.Context, r maps.RouteRequest) (maps.RouteResult, error) {
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, r)
	f.mu.Unlock()
	if f.wait != nil {
		<-f.wait
	}
	if f.err != nil {
		return maps.RouteResult{}, f.err
	}
	return maps.RouteResult{
		RouteLeg:     maps.RouteLeg{DistanceMeters: 1234, DurationSeconds: 320, StaticDurationSeconds: 300},
		Legs:         []maps.RouteLeg{{DistanceMeters: 1234, DurationSeconds: 320, StaticDurationSeconds: 300}},
		Polyline:     "_p~iF~ps|U_ulLnnqC_mqNvxq`@",
		Provider:     "google",
		CalculatedAt: time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC),
	}, nil
}

func (f *fakeRouteProvider) GetRouteMatrix(context.Context, maps.RouteMatrixRequest) (maps.RouteMatrixResult, error) {
	return maps.RouteMatrixResult{}, nil
}

func testActiveOrder(status string) *models.ActiveOrder {
	pickupLat, pickupLng := 30.71, 76.70
	dropLat, dropLng := 30.74, 76.78
	return &models.ActiveOrder{
		OrderID:         42,
		DeliveryStatus:  status,
		PickupLatitude:  &pickupLat,
		PickupLongitude: &pickupLng,
		DropLatitude:    &dropLat,
		DropLongitude:   &dropLng,
	}
}

func TestDeliveryRouteUsesServerResolvedDestination(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantKind   string
		wantLat    float64
		preference string
	}{
		{"before pickup", models.DeliveryStatusRiderAssigned, "pickup", 30.71, "TRAFFIC_AWARE"},
		{"after pickup", models.DeliveryStatusPickedUp, "drop", 30.74, "TRAFFIC_AWARE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &fakeRouteProvider{}
			svc := NewDeliveryRouteService(fakeActiveOrderResolver{order: testActiveOrder(tc.status)}, provider, DeliveryRouteOptions{
				TrafficAware: true,
				ResultTTL:    45 * time.Second,
			})
			route, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", maps.Coordinate{Latitude: 30.70, Longitude: 76.69})
			if err != nil {
				t.Fatal(err)
			}
			if route.DestinationType != tc.wantKind || route.DistanceMeters != 1234 || route.TrafficDelaySeconds == nil {
				t.Fatalf("unexpected route: %+v", route)
			}
			if got := provider.requests[0].Destination.Latitude; got != tc.wantLat {
				t.Fatalf("destination latitude got %v want %v", got, tc.wantLat)
			}
			if got := provider.requests[0].Options.RoutingPreference; got != tc.preference {
				t.Fatalf("preference got %s want %s", got, tc.preference)
			}
		})
	}
}

func TestDeliveryRouteRequiresMatchingActiveOrder(t *testing.T) {
	svc := NewDeliveryRouteService(fakeActiveOrderResolver{order: testActiveOrder(models.DeliveryStatusRiderAssigned)}, &fakeRouteProvider{}, DeliveryRouteOptions{})
	_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 7, "food", maps.Coordinate{Latitude: 30.70, Longitude: 76.69})
	var routeErr *DeliveryRouteError
	if !errors.As(err, &routeErr) || routeErr.Code != RouteErrOrderNotFound {
		t.Fatalf("got %v want %s", err, RouteErrOrderNotFound)
	}
}

func TestDeliveryRouteMapsNoActiveOrder(t *testing.T) {
	svc := NewDeliveryRouteService(fakeActiveOrderResolver{err: sql.ErrNoRows}, &fakeRouteProvider{}, DeliveryRouteOptions{})
	_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", maps.Coordinate{Latitude: 30.70, Longitude: 76.69})
	var routeErr *DeliveryRouteError
	if !errors.As(err, &routeErr) || routeErr.Code != RouteErrOrderNotFound {
		t.Fatalf("got %v want %s", err, RouteErrOrderNotFound)
	}
}

func TestDeliveryRouteDedupesConcurrentProviderCalls(t *testing.T) {
	wait := make(chan struct{})
	provider := &fakeRouteProvider{wait: wait}
	svc := NewDeliveryRouteService(fakeActiveOrderResolver{order: testActiveOrder(models.DeliveryStatusRiderAssigned)}, provider, DeliveryRouteOptions{})
	origin := maps.Coordinate{Latitude: 30.70, Longitude: 76.69}

	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() {
		_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", origin)
		first <- err
	}()
	for {
		provider.mu.Lock()
		calls := provider.calls
		provider.mu.Unlock()
		if calls == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	go func() {
		_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", origin)
		second <- err
	}()
	time.Sleep(20 * time.Millisecond)
	close(wait)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls != 1 {
		t.Fatalf("provider calls got %d want 1", provider.calls)
	}
}

func TestDeliveryRouteRateLimitsCompletedRefreshes(t *testing.T) {
	provider := &fakeRouteProvider{}
	svc := NewDeliveryRouteService(fakeActiveOrderResolver{order: testActiveOrder(models.DeliveryStatusRiderAssigned)}, provider, DeliveryRouteOptions{
		MinInterval: time.Minute,
	})
	origin := maps.Coordinate{Latitude: 30.70, Longitude: 76.69}
	if _, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", origin); err != nil {
		t.Fatal(err)
	}
	_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", origin)
	var routeErr *DeliveryRouteError
	if !errors.As(err, &routeErr) || routeErr.Code != RouteErrRateLimited {
		t.Fatalf("got %v want %s", err, RouteErrRateLimited)
	}
}

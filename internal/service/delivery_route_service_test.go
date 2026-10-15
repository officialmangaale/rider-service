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

// ---- fixtures ---------------------------------------------------------------

type mutableResolver struct {
	mu    sync.Mutex
	order *models.ActiveOrder
	err   error
}

func (m *mutableResolver) GetActiveOrder(context.Context, string) (*models.ActiveOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	cp := *m.order
	return &cp, nil
}

func (m *mutableResolver) setStatus(status string) {
	m.mu.Lock()
	m.order.DeliveryStatus = status
	m.mu.Unlock()
}

func (m *mutableResolver) setDrop(lat, lng float64) {
	m.mu.Lock()
	m.order.DropLatitude, m.order.DropLongitude = &lat, &lng
	m.mu.Unlock()
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
}
func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// geoRouteProvider answers with a straight route from the request's origin to
// its destination, so progress and deviation behave like a real polyline.
type geoRouteProvider struct {
	mu       sync.Mutex
	calls    int
	requests []maps.RouteRequest
	wait     chan struct{}
	err      error
	clock    *testClock
}

func (f *geoRouteProvider) GetRoute(_ context.Context, r maps.RouteRequest) (maps.RouteResult, error) {
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, r)
	wait, err := f.wait, f.err
	f.mu.Unlock()
	if wait != nil {
		<-wait
	}
	if err != nil {
		return maps.RouteResult{}, err
	}
	const steps = 20
	points := make([]maps.Coordinate, 0, steps+1)
	for i := 0; i <= steps; i++ {
		points = append(points, lerp(r.Origin, r.Destination, float64(i)/steps))
	}
	distance := int64(maps.DistanceMeters(r.Origin, r.Destination))
	duration := float64(distance) / 8.0 // ~29 km/h
	static := duration
	if r.Options.RoutingPreference == "TRAFFIC_AWARE" {
		static = duration * 0.9
	}
	calculated := time.Now().UTC()
	if f.clock != nil {
		calculated = f.clock.Now()
	}
	return maps.RouteResult{
		RouteLeg:     maps.RouteLeg{DistanceMeters: distance, DurationSeconds: duration, StaticDurationSeconds: static},
		Polyline:     maps.EncodePolyline(points),
		Provider:     "google",
		CalculatedAt: calculated,
	}, nil
}

func (f *geoRouteProvider) GetRouteMatrix(context.Context, maps.RouteMatrixRequest) (maps.RouteMatrixResult, error) {
	return maps.RouteMatrixResult{}, nil
}

func (f *geoRouteProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *geoRouteProvider) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func lerp(a, b maps.Coordinate, f float64) maps.Coordinate {
	return maps.Coordinate{
		Latitude:  a.Latitude + (b.Latitude-a.Latitude)*f,
		Longitude: a.Longitude + (b.Longitude-a.Longitude)*f,
	}
}

var (
	riderStart = maps.Coordinate{Latitude: 30.70, Longitude: 76.69}
	pickupAt   = maps.Coordinate{Latitude: 30.71, Longitude: 76.70}
	dropAt     = maps.Coordinate{Latitude: 30.74, Longitude: 76.78}
)

func testActiveOrder(status string) *models.ActiveOrder {
	pickupLat, pickupLng := pickupAt.Latitude, pickupAt.Longitude
	dropLat, dropLng := dropAt.Latitude, dropAt.Longitude
	return &models.ActiveOrder{
		OrderID:         42,
		DeliveryStatus:  status,
		PickupLatitude:  &pickupLat,
		PickupLongitude: &pickupLng,
		DropLatitude:    &dropLat,
		DropLongitude:   &dropLng,
	}
}

type routeHarness struct {
	svc      *DeliveryRouteService
	provider *geoRouteProvider
	resolver *mutableResolver
	clock    *testClock
}

func newHarness(status string, opts DeliveryRouteOptions) *routeHarness {
	clock := newTestClock()
	provider := &geoRouteProvider{clock: clock}
	resolver := &mutableResolver{order: testActiveOrder(status)}
	svc := NewDeliveryRouteService(resolver, provider, opts)
	svc.now = clock.Now
	return &routeHarness{svc: svc, provider: provider, resolver: resolver, clock: clock}
}

func (h *routeHarness) rider(origin maps.Coordinate) (*DeliveryRoute, error) {
	return h.svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", origin)
}

// customer is what a tracking poll does: the same call with the rider's last
// reported fix and its timestamp.
func (h *routeHarness) customer(origin maps.Coordinate, reportedAt time.Time) (*DeliveryRoute, error) {
	return h.svc.GetDeliveryRouteAt(context.Background(), "rider-1", 42, "food", origin, reportedAt)
}

func routeErrCode(t *testing.T, err error) string {
	t.Helper()
	var routeErr *DeliveryRouteError
	if !errors.As(err, &routeErr) {
		t.Fatalf("expected DeliveryRouteError, got %v", err)
	}
	return routeErr.Code
}

// ---- baseline behaviour (unchanged contract) ---------------------------------

func TestDeliveryRouteUsesServerResolvedDestination(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantKind   string
		wantLat    float64
		preference string
	}{
		{"before pickup", models.DeliveryStatusRiderAssigned, "pickup", pickupAt.Latitude, "TRAFFIC_AWARE"},
		{"at restaurant", models.DeliveryStatusRiderArrivedRestaurant, "pickup", pickupAt.Latitude, "TRAFFIC_AWARE"},
		{"after pickup", models.DeliveryStatusPickedUp, "drop", dropAt.Latitude, "TRAFFIC_AWARE"},
		{"on the way", models.DeliveryStatusOnTheWay, "drop", dropAt.Latitude, "TRAFFIC_AWARE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(tc.status, DeliveryRouteOptions{TrafficAware: true})
			route, err := h.rider(riderStart)
			if err != nil {
				t.Fatal(err)
			}
			if route.DestinationType != tc.wantKind || route.RouteDistanceMeters <= 0 || route.TrafficDelaySeconds == nil {
				t.Fatalf("unexpected route: %+v", route)
			}
			if got := h.provider.requests[0].Destination.Latitude; got != tc.wantLat {
				t.Fatalf("destination latitude got %v want %v", got, tc.wantLat)
			}
			if got := h.provider.requests[0].Options.RoutingPreference; got != tc.preference {
				t.Fatalf("preference got %s want %s", got, tc.preference)
			}
			if got := h.provider.requests[0].Options.TravelMode; got != "DRIVE" {
				t.Fatalf("travel mode got %s want DRIVE", got)
			}
		})
	}
}

func TestDefaultOptionsRequestEssentialsRouting(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	route, err := h.rider(riderStart)
	if err != nil {
		t.Fatal(err)
	}
	opts := h.provider.requests[0].Options
	if opts.RoutingPreference != "TRAFFIC_UNAWARE" || opts.TravelMode != "DRIVE" {
		t.Fatalf("default must be DRIVE/TRAFFIC_UNAWARE, got %+v", opts)
	}
	if maps.RoutesBillingTier(opts) != "essentials" {
		t.Fatal("default request must bill as Essentials")
	}
	if route.TrafficDelaySeconds != nil {
		t.Fatalf("no traffic data expected without traffic-aware routing: %+v", route)
	}
	if route.StaticDurationSeconds != route.DurationSeconds {
		t.Fatalf("static duration should equal duration: %+v", route)
	}
}

func TestDeliveryRouteRequiresMatchingActiveOrder(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	_, err := h.svc.GetDeliveryRoute(context.Background(), "rider-1", 7, "food", riderStart)
	if routeErrCode(t, err) != RouteErrOrderNotFound {
		t.Fatalf("got %v", err)
	}
	if h.provider.callCount() != 0 {
		t.Fatal("no provider call for a non-matching order")
	}
}

func TestDeliveryRouteMapsNoActiveOrder(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	h.resolver.err = sql.ErrNoRows
	_, err := h.rider(riderStart)
	if routeErrCode(t, err) != RouteErrOrderNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestDeliveryRouteRejectsInvalidOrigin(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	for _, origin := range []maps.Coordinate{{}, {Latitude: 91, Longitude: 10}, {Latitude: 10, Longitude: 181}} {
		_, err := h.rider(origin)
		if routeErrCode(t, err) != RouteErrInvalidCoordinates {
			t.Fatalf("origin %+v: got %v", origin, err)
		}
	}
	if h.provider.callCount() != 0 {
		t.Fatal("invalid origins must never reach the provider")
	}
}

// ---- reuse: the cost-critical behaviour --------------------------------------

func TestCustomerAndRiderPollsReuseOneRoute(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	first, err := h.rider(riderStart)
	if err != nil {
		t.Fatal(err)
	}
	if first.Reused || first.RefreshReason != RouteRefreshNoRoute {
		t.Fatalf("first request must calculate: %+v", first)
	}
	// 30 minutes of activity compressed: customer polls every 15 s and the rider
	// app every 30 s, with the rider moving steadily along the route.
	previousRemaining := first.DistanceMeters
	for i := 1; i <= 28; i++ {
		h.clock.Advance(15 * time.Second)
		position := lerp(riderStart, pickupAt, float64(i)/30)
		route, err := h.customer(position, h.clock.Now().Add(-3*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			if route, err = h.rider(position); err != nil {
				t.Fatal(err)
			}
		}
		if !route.Reused {
			t.Fatalf("poll %d recalculated the route: %+v", i, route)
		}
		if route.DistanceMeters > previousRemaining {
			t.Fatalf("remaining distance must not increase as the rider advances: %d > %d", route.DistanceMeters, previousRemaining)
		}
		previousRemaining = route.DistanceMeters
		if route.RouteDistanceMeters != first.RouteDistanceMeters || !route.GeneratedAt.Equal(first.GeneratedAt) {
			t.Fatalf("route identity changed: %+v", route)
		}
	}
	if got := h.provider.callCount(); got != 1 {
		t.Fatalf("provider calls got %d want 1 for the whole pickup leg", got)
	}
	if previousRemaining >= first.RouteDistanceMeters/2 {
		t.Fatalf("remaining %d should be well below the route total %d near the end", previousRemaining, first.RouteDistanceMeters)
	}
}

func TestRouteAgeAloneNeverTriggersAProviderCall(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{MaxRouteAge: 15 * time.Minute})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	for _, age := range []time.Duration{89 * time.Second, 91 * time.Second, 5 * time.Minute, 14 * time.Minute} {
		h.clock.Advance(age - (h.clock.Now().Sub(newTestClock().Now())))
		route, err := h.customer(riderStart, h.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !route.Reused {
			t.Fatalf("a route %s old with the rider on it must be reused: %+v", age, route)
		}
	}
	if got := h.provider.callCount(); got != 1 {
		t.Fatalf("provider calls got %d want 1", got)
	}
	// The hold limit is the one age-based trigger.
	h.clock.Advance(2 * time.Minute)
	route, err := h.customer(riderStart, h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if route.Reused || route.RefreshReason != RouteRefreshExpired {
		t.Fatalf("expected an expiry refresh: %+v", route)
	}
	if got := h.provider.callCount(); got != 2 {
		t.Fatalf("provider calls got %d want 2", got)
	}
}

func TestOrdinaryDeliveryMakesTwoProviderCalls(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	steps := []struct {
		status string
		from   maps.Coordinate
		to     maps.Coordinate
		polls  int
	}{
		{models.DeliveryStatusRiderAssigned, riderStart, pickupAt, 10},
		{models.DeliveryStatusRiderArrivedRestaurant, pickupAt, pickupAt, 3},
		{models.DeliveryStatusPickedUp, pickupAt, lerp(pickupAt, dropAt, 0.5), 12},
		{models.DeliveryStatusOnTheWay, lerp(pickupAt, dropAt, 0.5), dropAt, 10},
	}
	var kinds []string
	for _, step := range steps {
		h.resolver.setStatus(step.status)
		for i := 0; i < step.polls; i++ {
			h.clock.Advance(15 * time.Second)
			position := lerp(step.from, step.to, float64(i+1)/float64(step.polls))
			route, err := h.customer(position, h.clock.Now())
			if err != nil {
				t.Fatalf("%s poll %d: %v", step.status, i, err)
			}
			if len(kinds) == 0 || kinds[len(kinds)-1] != route.DestinationType {
				kinds = append(kinds, route.DestinationType)
			}
		}
	}
	if got := h.provider.callCount(); got != 2 {
		t.Fatalf("an ordinary delivery should need 2 provider calls (pickup leg + drop leg), got %d", got)
	}
	if len(kinds) != 2 || kinds[0] != "pickup" || kinds[1] != "drop" {
		t.Fatalf("destination sequence got %v", kinds)
	}
	// The second calculation must have started at the pickup, aimed at the drop.
	second := h.provider.requests[1]
	if second.Destination != dropAt {
		t.Fatalf("drop leg destination got %+v", second.Destination)
	}
}

func TestPickupRouteIsNeverServedForTheDropoffLeg(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	pickupRoute, err := h.rider(riderStart)
	if err != nil {
		t.Fatal(err)
	}
	h.resolver.setStatus(models.DeliveryStatusPickedUp)
	h.provider.setErr(&maps.RouteError{Code: maps.UpstreamError})
	h.clock.Advance(20 * time.Second)
	route, err := h.customer(pickupAt, h.clock.Now())
	if err == nil {
		t.Fatalf("a failed drop-off calculation must not fall back to the pickup route; got %+v (pickup polyline %s)", route, pickupRoute.EncodedPolyline)
	}
	if code := routeErrCode(t, err); code != RouteErrUpstream {
		t.Fatalf("got %s", code)
	}
	// Recovery: once the provider is healthy and the cooldown passed, the drop
	// route is calculated.
	h.provider.setErr(nil)
	h.clock.Advance(40 * time.Second)
	route, err = h.customer(pickupAt, h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if route.DestinationType != "drop" || route.RefreshReason != RouteRefreshPhaseChange {
		t.Fatalf("unexpected route after recovery: %+v", route)
	}
}

func TestDestinationChangeRefreshesTheRoute(t *testing.T) {
	h := newHarness(models.DeliveryStatusPickedUp, DeliveryRouteOptions{})
	if _, err := h.rider(pickupAt); err != nil {
		t.Fatal(err)
	}
	h.resolver.setDrop(30.745, 76.79)
	h.clock.Advance(30 * time.Second)
	route, err := h.rider(lerp(pickupAt, dropAt, 0.1))
	if err != nil {
		t.Fatal(err)
	}
	if route.Reused || route.RefreshReason != RouteRefreshDestinationChanged {
		t.Fatalf("expected destination_changed refresh: %+v", route)
	}
	if got := h.provider.callCount(); got != 2 {
		t.Fatalf("provider calls got %d want 2", got)
	}
}

// ---- deviation ----------------------------------------------------------------

func offRoutePoint(f float64) maps.Coordinate {
	// ~330 m north of the straight route.
	p := lerp(riderStart, pickupAt, f)
	p.Latitude += 0.003
	return p
}

func TestMaterialDeviationNeedsDistinctSamplesAndACooldown(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{
		OffRouteMeters: 100, OffRouteSamples: 2, DeviationCooldown: 45 * time.Second,
	})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	// The same off-route fix polled repeatedly is one sample, not many.
	h.clock.Advance(50 * time.Second)
	for i := 0; i < 5; i++ {
		route, err := h.customer(offRoutePoint(0.3), h.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if !route.Reused || !route.OffRoute {
			t.Fatalf("poll %d: want reused+off_route flag, got %+v", i, route)
		}
	}
	if h.provider.callCount() != 1 {
		t.Fatalf("one repeated sample must not refresh; calls=%d", h.provider.callCount())
	}
	// A second DISTINCT off-route fix confirms the deviation (cooldown has passed).
	route, err := h.customer(offRoutePoint(0.35), h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if route.Reused || route.RefreshReason != RouteRefreshOffRoute {
		t.Fatalf("expected off_route refresh: %+v", route)
	}
	if h.provider.callCount() != 2 {
		t.Fatalf("calls got %d want 2", h.provider.callCount())
	}
	// The recalculation starts from where the rider actually is.
	if got := h.provider.requests[1].Origin; got != offRoutePoint(0.35) {
		t.Fatalf("refresh origin got %+v", got)
	}
}

func TestOneOffRouteBlipDoesNotRefresh(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{OffRouteSamples: 2, DeviationCooldown: 5 * time.Second})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(60 * time.Second)
	for i, p := range []maps.Coordinate{
		offRoutePoint(0.2), lerp(riderStart, pickupAt, 0.25), offRoutePoint(0.3), lerp(riderStart, pickupAt, 0.35),
	} {
		h.clock.Advance(10 * time.Second)
		if _, err := h.customer(p, h.clock.Now()); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
	}
	if h.provider.callCount() != 1 {
		t.Fatalf("alternating blips must not refresh; calls=%d", h.provider.callCount())
	}
}

func TestDeviationBeforeCooldownIsDeferred(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{OffRouteSamples: 2, DeviationCooldown: 45 * time.Second})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(10 * time.Second)
	_, _ = h.customer(offRoutePoint(0.2), h.clock.Now())
	h.clock.Advance(10 * time.Second)
	if _, err := h.customer(offRoutePoint(0.25), h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if h.provider.callCount() != 1 {
		t.Fatalf("refresh within the deviation cooldown must wait; calls=%d", h.provider.callCount())
	}
	h.clock.Advance(40 * time.Second)
	route, err := h.customer(offRoutePoint(0.3), h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if route.RefreshReason != RouteRefreshOffRoute || h.provider.callCount() != 2 {
		t.Fatalf("expected deferred off_route refresh: %+v calls=%d", route, h.provider.callCount())
	}
}

// ---- failures, cooldowns, budgets ---------------------------------------------

func TestDeviationRefreshFailureKeepsTheExistingRoute(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{
		OffRouteSamples: 2, DeviationCooldown: 5 * time.Second, FailureCooldown: 30 * time.Second,
	})
	original, err := h.rider(riderStart)
	if err != nil {
		t.Fatal(err)
	}
	h.provider.setErr(&maps.RouteError{Code: maps.Timeout})
	h.clock.Advance(20 * time.Second)
	_, _ = h.customer(offRoutePoint(0.2), h.clock.Now())
	h.clock.Advance(10 * time.Second)
	route, err := h.customer(offRoutePoint(0.25), h.clock.Now())
	if err != nil {
		t.Fatalf("a failed deviation refresh should still show the existing route: %v", err)
	}
	if route.FallbackReason != "refresh_failed" || route.EncodedPolyline != original.EncodedPolyline {
		t.Fatalf("expected fallback to the existing route: %+v", route)
	}
	callsAfterFailure := h.provider.callCount()
	// During the failure cooldown nothing may hammer the provider.
	for i := 0; i < 5; i++ {
		h.clock.Advance(3 * time.Second)
		route, err = h.customer(offRoutePoint(0.26+float64(i)/100), h.clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if route.FallbackReason != "failure_cooldown" {
			t.Fatalf("poll %d fallback got %q", i, route.FallbackReason)
		}
	}
	if h.provider.callCount() != callsAfterFailure {
		t.Fatalf("provider called during failure cooldown: %d -> %d", callsAfterFailure, h.provider.callCount())
	}
}

func TestInitialFailureIsReportedAndRetriesAreSpaced(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{FailureCooldown: 30 * time.Second})
	h.provider.setErr(&maps.RouteError{Code: maps.UpstreamError})
	_, err := h.customer(riderStart, h.clock.Now())
	if routeErrCode(t, err) != RouteErrUpstream {
		t.Fatalf("got %v", err)
	}
	for i := 0; i < 4; i++ { // customer polls every 5 s while a rider is awaited
		h.clock.Advance(5 * time.Second)
		if _, err := h.customer(riderStart, h.clock.Now()); err == nil {
			t.Fatal("expected the cooled-down failure to be returned")
		}
	}
	if h.provider.callCount() != 1 {
		t.Fatalf("provider calls got %d want 1 during cooldown", h.provider.callCount())
	}
	h.provider.setErr(nil)
	h.clock.Advance(15 * time.Second)
	route, err := h.customer(riderStart, h.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if route.Reused || h.provider.callCount() != 2 {
		t.Fatalf("expected one retry after the cooldown: %+v calls=%d", route, h.provider.callCount())
	}
}

func TestPerMinuteBudgetBoundsProviderCalls(t *testing.T) {
	clock := newTestClock()
	provider := &geoRouteProvider{clock: clock}
	resolver := &mutableResolver{order: testActiveOrder(models.DeliveryStatusRiderAssigned)}
	svc := NewDeliveryRouteService(resolver, provider, DeliveryRouteOptions{MaxCallsPerMinute: 2})
	svc.now = clock.Now
	for orderID := 1; orderID <= 3; orderID++ {
		resolver.order.OrderID = orderID
		_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", orderID, "food", riderStart)
		if orderID <= 2 && err != nil {
			t.Fatalf("order %d: %v", orderID, err)
		}
		if orderID == 3 {
			if routeErrCode(t, err) != RouteErrBudgetExceeded {
				t.Fatalf("third call within a minute got %v", err)
			}
		}
	}
	if provider.callCount() != 2 {
		t.Fatalf("calls got %d want 2", provider.callCount())
	}
	clock.Advance(61 * time.Second)
	if _, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 3, "food", riderStart); err != nil {
		t.Fatalf("budget window should have reset: %v", err)
	}
}

func TestDailyBudgetAndPerOrderCap(t *testing.T) {
	// Per-order cap: pickup leg uses the single allowed call; the drop leg is
	// refused rather than served from the pickup route.
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{MaxCallsPerOrder: 1})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	h.resolver.setStatus(models.DeliveryStatusPickedUp)
	h.clock.Advance(time.Minute)
	_, err := h.rider(pickupAt)
	if routeErrCode(t, err) != RouteErrBudgetExceeded {
		t.Fatalf("got %v", err)
	}
	if h.provider.callCount() != 1 {
		t.Fatalf("calls got %d want 1", h.provider.callCount())
	}

	daily := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{DailyBudget: 1})
	if _, err := daily.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	daily.resolver.setStatus(models.DeliveryStatusPickedUp)
	daily.clock.Advance(time.Minute)
	if _, err := daily.rider(pickupAt); routeErrCode(t, err) != RouteErrBudgetExceeded {
		t.Fatalf("daily budget got %v", err)
	}
	daily.clock.Advance(24 * time.Hour)
	if _, err := daily.rider(pickupAt); err != nil {
		t.Fatalf("daily budget should reset on a new day: %v", err)
	}
}

func TestExpiredRouteIsNotServedWhenRefreshIsRefused(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{MaxRouteAge: time.Minute, MaxCallsPerOrder: 1})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(2 * time.Minute)
	route, err := h.rider(riderStart)
	if err == nil {
		t.Fatalf("an expired route must not be served when it cannot be refreshed: %+v", route)
	}
}

func TestConcurrentRequestsShareOneProviderCall(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	wait := make(chan struct{})
	h.provider.wait = wait
	origins := []maps.Coordinate{riderStart, lerp(riderStart, pickupAt, 0.01), lerp(riderStart, pickupAt, 0.02)}
	errs := make(chan error, len(origins))
	for i, origin := range origins {
		i, origin := i, origin
		go func() {
			var err error
			if i%2 == 0 {
				_, err = h.rider(origin)
			} else {
				_, err = h.customer(origin, time.Time{})
			}
			errs <- err
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for h.provider.callCount() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("provider never called")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	close(wait)
	for range origins {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if got := h.provider.callCount(); got != 1 {
		t.Fatalf("provider calls got %d want 1 (singleflight)", got)
	}
}

// ---- lifecycle, isolation, ETA ---------------------------------------------------

func TestRouteIsDroppedWhenTheOrderEnds(t *testing.T) {
	h := newHarness(models.DeliveryStatusOnTheWay, DeliveryRouteOptions{})
	if _, err := h.rider(pickupAt); err != nil {
		t.Fatal(err)
	}
	h.resolver.setStatus(models.DeliveryStatusDelivered)
	_, err := h.rider(dropAt)
	if routeErrCode(t, err) != RouteErrOrderNotFound {
		t.Fatalf("got %v", err)
	}
	h.svc.mu.Lock()
	remaining := len(h.svc.sessions)
	h.svc.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("route data must not outlive the order, %d sessions remain", remaining)
	}
}

func TestExpiredSessionsArePruned(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{MaxRouteAge: time.Minute})
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(3 * time.Minute)
	// Any later request for another order triggers the sweep.
	h.resolver.order.OrderID = 99
	_, _ = h.svc.GetDeliveryRoute(context.Background(), "rider-2", 99, "food", riderStart)
	h.svc.mu.Lock()
	_, stale := h.svc.sessions[sessionKey("rider-1", 42)]
	h.svc.mu.Unlock()
	if stale {
		t.Fatal("an expired session must be removed by the sweep")
	}
}

func TestRoutesAreIsolatedPerOrderAndRider(t *testing.T) {
	clock := newTestClock()
	provider := &geoRouteProvider{clock: clock}
	resolver := &mutableResolver{order: testActiveOrder(models.DeliveryStatusRiderAssigned)}
	svc := NewDeliveryRouteService(resolver, provider, DeliveryRouteOptions{})
	svc.now = clock.Now
	ctx := context.Background()
	if _, err := svc.GetDeliveryRoute(ctx, "rider-1", 42, "food", riderStart); err != nil {
		t.Fatal(err)
	}
	// Another rider on the same order id must not receive rider-1's route.
	route, err := svc.GetDeliveryRoute(ctx, "rider-2", 42, "food", riderStart)
	if err != nil {
		t.Fatal(err)
	}
	if route.Reused || provider.callCount() != 2 {
		t.Fatalf("rider-2 must calculate its own route: %+v calls=%d", route, provider.callCount())
	}
}

func TestRemainingETAFollowsProgressAndTimestampsStaySeparate(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	first, err := h.rider(riderStart)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(4 * time.Minute)
	reported := h.clock.Now().Add(-20 * time.Second)
	halfway, err := h.customer(lerp(riderStart, pickupAt, 0.5), reported)
	if err != nil {
		t.Fatal(err)
	}
	if halfway.ETABasis != routeETABasisProgress {
		t.Fatalf("basis got %s", halfway.ETABasis)
	}
	ratio := halfway.DurationSeconds / first.RouteDurationSeconds
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("ETA at the midpoint should be about half the route: %.2f", ratio)
	}
	if !halfway.GeneratedAt.Equal(first.GeneratedAt) {
		t.Fatal("route calculation time must not change on reuse")
	}
	if !halfway.ETACalculatedAt.Equal(h.clock.Now()) {
		t.Fatalf("eta calculated at %v want %v", halfway.ETACalculatedAt, h.clock.Now())
	}
	if !halfway.RiderLocationAt.Equal(reported) {
		t.Fatalf("rider location time %v want %v", halfway.RiderLocationAt, reported)
	}
	if !halfway.ExpiresAt.Equal(first.GeneratedAt.Add(15 * time.Minute)) {
		t.Fatalf("hold limit got %v", halfway.ExpiresAt)
	}
}

func TestRestartRecalculatesOnceThenReuses(t *testing.T) {
	h := newHarness(models.DeliveryStatusOnTheWay, DeliveryRouteOptions{})
	if _, err := h.rider(pickupAt); err != nil {
		t.Fatal(err)
	}
	// A restart loses in-memory routes; a reconnecting client costs one call.
	fresh := NewDeliveryRouteService(h.resolver, h.provider, DeliveryRouteOptions{})
	fresh.now = h.clock.Now
	for i := 0; i < 5; i++ {
		h.clock.Advance(15 * time.Second)
		if _, err := fresh.GetDeliveryRouteAt(context.Background(), "rider-1", 42, "food", lerp(pickupAt, dropAt, 0.1+float64(i)/20), time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.provider.callCount(); got != 2 {
		t.Fatalf("provider calls got %d want 2 (original + one after restart)", got)
	}
}

func TestUsageCountersRecordReuseAndRefreshReasons(t *testing.T) {
	h := newHarness(models.DeliveryStatusRiderAssigned, DeliveryRouteOptions{})
	usage := maps.NewUsage()
	h.svc.usage = usage
	if _, err := h.rider(riderStart); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		h.clock.Advance(10 * time.Second)
		if _, err := h.customer(riderStart, h.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	snap := usage.Snapshot()
	if snap["routes.refresh.no_route"] != 1 || snap["routes.reused"] != 4 {
		t.Fatalf("unexpected counters: %v", snap)
	}
}

func TestCustomerTrackingDTOCarriesRemainingETA(t *testing.T) {
	now := time.Now().UTC()
	dto := customerTrackingRouteDTO(&DeliveryRoute{
		EncodedPolyline: "x", DistanceMeters: 800, DurationSeconds: 130,
		RouteDistanceMeters: 2000, RouteDurationSeconds: 400,
		GeneratedAt: now.Add(-5 * time.Minute), ExpiresAt: now.Add(10 * time.Minute),
		ETACalculatedAt: now, RiderLocationAt: now.Add(-4 * time.Second),
		ETABasis: routeETABasisProgress, Reused: true, DestinationType: "drop",
	})
	if dto.ETAMinutes != 3 || dto.ETASeconds != 130 || dto.DistanceMeters != 800 || dto.RouteDurationSeconds != 400 {
		t.Fatalf("unexpected dto: %+v", dto)
	}
	if dto.Stale || dto.ETABasis != routeETABasisProgress || dto.ETACalculatedAt == "" || dto.RiderLocationAt == "" {
		t.Fatalf("unexpected dto metadata: %+v", dto)
	}
	if customerTrackingRouteDTO(nil) != nil {
		t.Fatal("nil route must stay nil")
	}
}

type panickingRouteProvider struct{}

func (panickingRouteProvider) GetRoute(context.Context, maps.RouteRequest) (maps.RouteResult, error) {
	panic("provider bug")
}

func (panickingRouteProvider) GetRouteMatrix(context.Context, maps.RouteMatrixRequest) (maps.RouteMatrixResult, error) {
	return maps.RouteMatrixResult{}, nil
}

// A provider panic must neither leak the concurrency slot nor leave the
// order's in-flight entry behind, or that order could never get a route again.
func TestProviderPanicDoesNotWedgeTheOrder(t *testing.T) {
	clock := newTestClock()
	resolver := &mutableResolver{order: testActiveOrder(models.DeliveryStatusRiderAssigned)}
	svc := NewDeliveryRouteService(resolver, panickingRouteProvider{}, DeliveryRouteOptions{MaxConcurrent: 1, FailureCooldown: 5 * time.Second})
	svc.now = clock.Now
	_, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", riderStart)
	if routeErrCode(t, err) != RouteErrUpstream {
		t.Fatalf("got %v", err)
	}
	svc.mu.Lock()
	inflight := len(svc.inflight)
	svc.mu.Unlock()
	if inflight != 0 || len(svc.sem) != 0 {
		t.Fatalf("leaked state: inflight=%d slots=%d", inflight, len(svc.sem))
	}
	// After the cooldown the order is served by a healthy provider.
	healthy := &geoRouteProvider{clock: clock}
	svc.provider = healthy
	clock.Advance(10 * time.Second)
	if _, err := svc.GetDeliveryRoute(context.Background(), "rider-1", 42, "food", riderStart); err != nil {
		t.Fatalf("order must recover: %v", err)
	}
}

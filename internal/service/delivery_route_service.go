package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/maps"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

const (
	RouteErrOrderNotFound      = "ROUTE_ORDER_NOT_FOUND"
	RouteErrInvalidCoordinates = "ROUTE_INVALID_COORDINATES"
	RouteErrProviderDisabled   = "ROUTE_PROVIDER_DISABLED"
	RouteErrNotConfigured      = "ROUTE_NOT_CONFIGURED"
	RouteErrNoRoute            = "ROUTE_NOT_FOUND"
	RouteErrRateLimited        = "ROUTE_RATE_LIMITED"
	RouteErrBudgetExceeded     = "ROUTE_BUDGET_EXCEEDED"
	RouteErrUpstream           = "ROUTE_UPSTREAM_UNAVAILABLE"
)

// Why a route was calculated. These are the only reasons a provider request
// is made; route age below the hold limit is deliberately not one of them.
const (
	RouteRefreshNoRoute            = "no_route"
	RouteRefreshPhaseChange        = "phase_change"
	RouteRefreshDestinationChanged = "destination_changed"
	RouteRefreshOffRoute           = "off_route"
	RouteRefreshExpired            = "expired"
)

const (
	routeETABasisProgress = "route_progress"
	routeETABasisOriginal = "route_original"

	routePruneInterval = time.Minute
	routeMaxSessions   = 5000
)

type activeOrderResolver interface {
	GetActiveOrder(context.Context, string) (*models.ActiveOrder, error)
}

type DeliveryRouteOptions struct {
	// TrafficAware requests Routes Pro. Off by default (Essentials).
	TrafficAware bool
	// MaxRouteAge is the hold limit for an order's route. A route older than
	// this is recalculated; younger routes are reused while the rider stays on
	// them. It also bounds how long route data is kept in memory.
	MaxRouteAge time.Duration
	// MinInterval spaces provider calls for one order and destination.
	MinInterval time.Duration
	// OffRouteMeters/OffRouteSamples define a material deviation: the rider is
	// further than OffRouteMeters from the route on OffRouteSamples distinct
	// consecutive position samples.
	OffRouteMeters  float64
	OffRouteSamples int
	// DeviationCooldown is the minimum route age before a deviation recalculates.
	DeviationCooldown time.Duration
	// FailureCooldown suppresses retries after a provider failure.
	FailureCooldown time.Duration
	// Budgets. All are in-process and bound spend even if a client misbehaves.
	MaxCallsPerOrder  int
	MaxCallsPerMinute int
	DailyBudget       int
	MaxConcurrent     int
}

func (o DeliveryRouteOptions) withDefaults() DeliveryRouteOptions {
	if o.MaxRouteAge <= 0 || o.MaxRouteAge > 30*time.Minute {
		o.MaxRouteAge = 15 * time.Minute
	}
	if o.MinInterval <= 0 {
		o.MinInterval = 5 * time.Second
	}
	if o.OffRouteMeters <= 0 {
		o.OffRouteMeters = 100
	}
	if o.OffRouteSamples <= 0 {
		o.OffRouteSamples = 2
	}
	if o.DeviationCooldown <= 0 {
		o.DeviationCooldown = 45 * time.Second
	}
	if o.FailureCooldown <= 0 {
		o.FailureCooldown = 30 * time.Second
	}
	if o.MaxCallsPerOrder <= 0 {
		o.MaxCallsPerOrder = 8
	}
	if o.MaxCallsPerMinute <= 0 {
		o.MaxCallsPerMinute = 30
	}
	if o.DailyBudget <= 0 {
		o.DailyBudget = 2000
	}
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = 4
	}
	return o
}

// DeliveryRoute is what clients receive. Three timestamps are kept distinct:
//
//	GeneratedAt     when the provider calculated the route
//	ETACalculatedAt when RemainingDistance/Duration were derived from it
//	RiderLocationAt when the rider position used for that derivation was reported
//
// DistanceMeters and DurationSeconds are the distance and time remaining along
// the route from the rider's position. They are derived from the provider's
// route, scaled by the share of the polyline still ahead; ETABasis says so.
// RouteDistanceMeters and RouteDurationSeconds are the provider's own totals.
type DeliveryRoute struct {
	EncodedPolyline       string    `json:"encoded_polyline"`
	DistanceMeters        int64     `json:"distance_meters"`
	DurationSeconds       float64   `json:"duration_seconds"`
	StaticDurationSeconds float64   `json:"static_duration_seconds"`
	TrafficDelaySeconds   *float64  `json:"traffic_delay_seconds,omitempty"`
	Provider              string    `json:"provider"`
	DestinationType       string    `json:"destination_type"`
	GeneratedAt           time.Time `json:"generated_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	FallbackReason        string    `json:"fallback_reason,omitempty"`

	RouteDistanceMeters  int64     `json:"route_distance_meters"`
	RouteDurationSeconds float64   `json:"route_duration_seconds"`
	ETACalculatedAt      time.Time `json:"eta_calculated_at"`
	RiderLocationAt      time.Time `json:"rider_location_at"`
	ETABasis             string    `json:"eta_basis"`
	RefreshReason        string    `json:"refresh_reason,omitempty"`
	Reused               bool      `json:"reused"`
	OffRoute             bool      `json:"off_route,omitempty"`
}

type DeliveryRouteError struct {
	Code string
	Err  error
}

func (e *DeliveryRouteError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}
func (e *DeliveryRouteError) Unwrap() error { return e.Err }

type routeDestination struct {
	kind       string
	coordinate maps.Coordinate
}

// routeSession is one order's current route for one rider and destination. It
// is held in memory only, replaced on every phase change, and dropped when the
// order ends or MaxRouteAge passes. It is never written to a database.
type routeSession struct {
	orderID      int
	riderID      string
	destKind     string
	dest         maps.Coordinate
	polyline     string
	points       []maps.Coordinate
	distance     int64
	duration     float64
	static       float64
	trafficDelay *float64
	provider     string
	calculated   time.Time
	reason       string

	// lastSample/offStreak implement deviation detection. A sample counts once
	// however many polls observe it, so repeated polls cannot fake a deviation.
	lastSample string
	offStreak  int
}

type routeCall struct {
	done    chan struct{}
	session *routeSession
	err     error
}

type routeFailure struct {
	at   time.Time
	code string
}

type orderCallCount struct {
	count int
	last  time.Time
}

type DeliveryRouteService struct {
	active   activeOrderResolver
	provider maps.RouteProvider
	options  DeliveryRouteOptions
	usage    *maps.Usage
	sem      chan struct{}
	now      func() time.Time

	mu          sync.Mutex
	sessions    map[string]*routeSession
	inflight    map[string]*routeCall
	attempts    map[string]time.Time
	failures    map[string]routeFailure
	orderCalls  map[int]*orderCallCount
	minuteStart time.Time
	minuteCount int
	dayKey      string
	dayCount    int
	lastPrune   time.Time
}

func NewDeliveryRouteService(active activeOrderResolver, provider maps.RouteProvider, options DeliveryRouteOptions) *DeliveryRouteService {
	options = options.withDefaults()
	return &DeliveryRouteService{
		active:     active,
		provider:   provider,
		options:    options,
		usage:      maps.DefaultUsage,
		sem:        make(chan struct{}, options.MaxConcurrent),
		now:        time.Now,
		sessions:   make(map[string]*routeSession),
		inflight:   make(map[string]*routeCall),
		attempts:   make(map[string]time.Time),
		failures:   make(map[string]routeFailure),
		orderCalls: make(map[int]*orderCallCount),
	}
}

// GetDeliveryRoute is the rider-app entry point. The origin is the rider's
// current position as reported by the rider app.
func (s *DeliveryRouteService) GetDeliveryRoute(ctx context.Context, riderID string, orderID int, orderType string, origin maps.Coordinate) (*DeliveryRoute, error) {
	return s.GetDeliveryRouteAt(ctx, riderID, orderID, orderType, origin, time.Time{})
}

// GetDeliveryRouteAt returns the route for the rider's active order. It makes
// a provider request only when one of the refresh triggers holds; otherwise it
// answers from the order's existing route, re-deriving remaining distance and
// ETA from the rider's position. originAt is when that position was reported
// (zero means "now").
//
// The customer tracking snapshot calls this on every poll. That is safe by
// construction: a poll that finds a valid route is a memory read.
func (s *DeliveryRouteService) GetDeliveryRouteAt(ctx context.Context, riderID string, orderID int, orderType string, origin maps.Coordinate, originAt time.Time) (*DeliveryRoute, error) {
	if s == nil || s.active == nil || s.provider == nil {
		return nil, &DeliveryRouteError{Code: RouteErrProviderDisabled}
	}
	if strings.TrimSpace(riderID) == "" || orderID <= 0 {
		return nil, &DeliveryRouteError{Code: RouteErrOrderNotFound}
	}
	if !origin.Valid() {
		return nil, &DeliveryRouteError{Code: RouteErrInvalidCoordinates}
	}
	key := sessionKey(riderID, orderID)
	active, err := s.active.GetActiveOrder(ctx, riderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.forget(key)
			return nil, &DeliveryRouteError{Code: RouteErrOrderNotFound, Err: err}
		}
		return nil, &DeliveryRouteError{Code: RouteErrUpstream, Err: err}
	}
	if active == nil || active.OrderID != orderID {
		return nil, &DeliveryRouteError{Code: RouteErrOrderNotFound}
	}
	dest, err := deliveryRouteDestination(active)
	if err != nil {
		var routeErr *DeliveryRouteError
		if errors.As(err, &routeErr) && routeErr.Code == RouteErrOrderNotFound {
			s.forget(key) // delivered/cancelled: nothing further to show or keep
		}
		return nil, err
	}
	now := s.now()
	if originAt.IsZero() || originAt.After(now) {
		originAt = now
	}

	existing, reason := s.evaluate(key, dest, origin, now)
	if reason == "" {
		s.usage.Inc("routes.reused")
		route := s.render(existing, origin, originAt, now, "", true)
		s.observe(riderID, orderID, dest.kind, "", true, nil)
		return route, nil
	}
	return s.refresh(ctx, key, reason, existing, dest, origin, originAt, orderID, riderID)
}

// evaluate decides whether the order's current route may be reused. It
// returns the existing session (possibly nil) and the refresh reason, which is
// empty when the route should simply be reused.
func (s *DeliveryRouteService) evaluate(key string, dest routeDestination, origin maps.Coordinate, now time.Time) (*routeSession, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Take this order's session before sweeping, so a session that has just
	// outlived its hold limit is classified as expired rather than missing.
	sess := s.sessions[key]
	s.pruneLocked(now)
	switch {
	case sess == nil:
		return nil, RouteRefreshNoRoute
	case sess.destKind != dest.kind:
		// Never reuse the pickup route for the drop-off leg (or vice versa).
		return sess, RouteRefreshPhaseChange
	case !sameCoordinate(sess.dest, dest.coordinate):
		return sess, RouteRefreshDestinationChanged
	case now.Sub(sess.calculated) >= s.options.MaxRouteAge:
		return sess, RouteRefreshExpired
	}
	progress, ok := maps.ProjectOnPolyline(sess.points, origin)
	if sample := sampleKey(origin); sample != sess.lastSample {
		sess.lastSample = sample
		if ok && progress.OffsetMeters > s.options.OffRouteMeters {
			sess.offStreak++
		} else {
			sess.offStreak = 0
		}
	}
	if sess.offStreak >= s.options.OffRouteSamples && now.Sub(sess.calculated) >= s.options.DeviationCooldown {
		return sess, RouteRefreshOffRoute
	}
	return sess, ""
}

func (s *DeliveryRouteService) refresh(ctx context.Context, key, reason string, existing *routeSession, dest routeDestination, origin maps.Coordinate, originAt time.Time, orderID int, riderID string) (*DeliveryRoute, error) {
	singleKey := key + ":" + dest.kind
	now := s.now()

	s.mu.Lock()
	if call := s.inflight[singleKey]; call != nil {
		s.mu.Unlock()
		s.usage.Inc("routes.deduped")
		return s.await(ctx, call, origin, originAt)
	}
	// Another request may have finished a refresh between our evaluation and
	// taking the lock. If the order's route was replaced with one for this
	// destination, use it instead of asking the provider again.
	if cur := s.sessions[key]; cur != nil && cur != existing && cur.destKind == dest.kind && sameCoordinate(cur.dest, dest.coordinate) {
		s.mu.Unlock()
		s.usage.Inc("routes.deduped")
		return s.render(cur, origin, originAt, now, "", true), nil
	}
	if denied, code := s.admitLocked(singleKey, orderID, reason, now); denied != "" {
		s.mu.Unlock()
		return s.refuse(key, reason, denied, code, existing, dest, origin, originAt, orderID, riderID)
	}
	call := &routeCall{done: make(chan struct{})}
	s.inflight[singleKey] = call
	s.mu.Unlock()

	result, err := s.computeReleasing(ctx, origin, dest)

	finished := s.now()
	s.mu.Lock()
	delete(s.inflight, singleKey)
	var created *routeSession
	if err == nil {
		created = &routeSession{
			orderID: orderID, riderID: riderID, destKind: dest.kind, dest: dest.coordinate,
			polyline: result.polyline, points: result.points, distance: result.distance,
			duration: result.duration, static: result.static, trafficDelay: result.trafficDelay,
			provider: result.provider, calculated: result.calculated, reason: reason,
			lastSample: sampleKey(origin),
		}
		if created.calculated.IsZero() || created.calculated.After(finished) {
			created.calculated = finished.UTC()
		}
		s.sessions[key] = created
		delete(s.failures, singleKey)
	} else {
		s.failures[singleKey] = routeFailure{at: finished, code: errorCode(err)}
	}
	count := 0
	if oc := s.orderCalls[orderID]; oc != nil {
		count = oc.count
	}
	s.mu.Unlock()

	call.session, call.err = created, err
	close(call.done)

	if err != nil {
		s.usage.Inc("routes.error." + strings.ToLower(errorCode(err)))
		s.observeCalls(riderID, orderID, dest.kind, reason, false, err, count)
		// A failed deviation refresh keeps showing the route we already have:
		// it is still inside its hold limit and the destination is unchanged.
		if reason == RouteRefreshOffRoute && existing != nil {
			s.usage.Inc("routes.fallback")
			return s.render(existing, origin, originAt, now, "refresh_failed", true), nil
		}
		return nil, err
	}
	s.usage.Inc("routes.refresh." + reason)
	s.observeCalls(riderID, orderID, dest.kind, reason, false, nil, count)
	return s.render(created, origin, originAt, finished, "", false), nil
}

// admitLocked applies cooldowns and budgets, and acquires a concurrency slot
// when it admits. It returns the kind of refusal, or "" when admitted.
func (s *DeliveryRouteService) admitLocked(singleKey string, orderID int, reason string, now time.Time) (string, string) {
	if f, ok := s.failures[singleKey]; ok && now.Sub(f.at) < s.options.FailureCooldown {
		return "failure_cooldown", f.code
	}
	if last, ok := s.attempts[singleKey]; ok && now.Sub(last) < s.options.MinInterval {
		return "min_interval", RouteErrRateLimited
	}
	oc := s.orderCalls[orderID]
	if oc != nil && oc.count >= s.options.MaxCallsPerOrder {
		return "order_cap", RouteErrBudgetExceeded
	}
	if s.minuteStart.IsZero() || now.Sub(s.minuteStart) >= time.Minute {
		s.minuteStart, s.minuteCount = now, 0
	}
	if s.minuteCount >= s.options.MaxCallsPerMinute {
		return "minute_budget", RouteErrBudgetExceeded
	}
	if day := now.UTC().Format("2006-01-02"); day != s.dayKey {
		s.dayKey, s.dayCount = day, 0
	}
	if s.dayCount >= s.options.DailyBudget {
		return "daily_budget", RouteErrBudgetExceeded
	}
	select {
	case s.sem <- struct{}{}:
	default:
		return "concurrency", RouteErrBudgetExceeded
	}
	s.attempts[singleKey] = now
	s.minuteCount++
	s.dayCount++
	if oc == nil {
		oc = &orderCallCount{}
		s.orderCalls[orderID] = oc
	}
	oc.count++
	oc.last = now
	return "", ""
}

// refuse answers a refresh that a cooldown or budget declined. Only a
// deviation refresh may fall back to the existing route: a missing, wrong-
// destination or expired route is never served in place of a fresh one.
func (s *DeliveryRouteService) refuse(key, reason, denied, code string, existing *routeSession, dest routeDestination, origin maps.Coordinate, originAt time.Time, orderID int, riderID string) (*DeliveryRoute, error) {
	s.usage.Inc("routes.denied." + denied)
	s.observe(riderID, orderID, dest.kind, reason, false, &DeliveryRouteError{Code: code})
	if reason == RouteRefreshOffRoute && existing != nil {
		s.usage.Inc("routes.fallback")
		return s.render(existing, origin, originAt, s.now(), denied, true), nil
	}
	if reason == RouteRefreshExpired {
		s.forget(key)
	}
	return nil, &DeliveryRouteError{Code: code}
}

func (s *DeliveryRouteService) await(ctx context.Context, call *routeCall, origin maps.Coordinate, originAt time.Time) (*DeliveryRoute, error) {
	select {
	case <-call.done:
	case <-ctx.Done():
		return nil, &DeliveryRouteError{Code: RouteErrUpstream, Err: ctx.Err()}
	}
	if call.err != nil {
		return nil, call.err
	}
	return s.render(call.session, origin, originAt, s.now(), "", false), nil
}

// computeReleasing runs the provider call and always returns the concurrency
// slot taken by admitLocked. A provider panic becomes an upstream error, so it
// can neither leak the slot nor leave the order's in-flight entry behind.
func (s *DeliveryRouteService) computeReleasing(ctx context.Context, origin maps.Coordinate, destination routeDestination) (result *computedRoute, err error) {
	defer func() { <-s.sem }()
	defer func() {
		if recovered := recover(); recovered != nil {
			result, err = nil, &DeliveryRouteError{Code: RouteErrUpstream, Err: fmt.Errorf("route provider panic: %v", recovered)}
		}
	}()
	return s.compute(ctx, origin, destination)
}

type computedRoute struct {
	polyline     string
	points       []maps.Coordinate
	distance     int64
	duration     float64
	static       float64
	trafficDelay *float64
	provider     string
	calculated   time.Time
}

func (s *DeliveryRouteService) compute(ctx context.Context, origin maps.Coordinate, destination routeDestination) (*computedRoute, error) {
	preference := "TRAFFIC_UNAWARE"
	if s.options.TrafficAware {
		preference = "TRAFFIC_AWARE"
	}
	result, err := s.provider.GetRoute(ctx, maps.RouteRequest{
		Origin:      origin,
		Destination: destination.coordinate,
		Options: maps.RouteOptions{
			TravelMode:        "DRIVE",
			RoutingPreference: preference,
			IncludePolyline:   true,
		},
	})
	if err != nil {
		return nil, mapRouteProviderError(err)
	}
	if result.Polyline == "" || result.DistanceMeters < 0 || result.DurationSeconds < 0 {
		return nil, &DeliveryRouteError{Code: RouteErrUpstream}
	}
	points, decodeErr := maps.DecodePolyline(result.Polyline)
	if decodeErr != nil {
		return nil, &DeliveryRouteError{Code: RouteErrUpstream, Err: decodeErr}
	}
	static := result.StaticDurationSeconds
	if static <= 0 {
		static = result.DurationSeconds
	}
	var delay *float64
	if result.DurationSeconds > static {
		value := result.DurationSeconds - static
		delay = &value
	}
	return &computedRoute{
		polyline: result.Polyline, points: points, distance: result.DistanceMeters,
		duration: result.DurationSeconds, static: static, trafficDelay: delay,
		provider: result.Provider, calculated: result.CalculatedAt,
	}, nil
}

// render derives what a client sees from a session and the rider's position.
func (s *DeliveryRouteService) render(sess *routeSession, origin maps.Coordinate, originAt, now time.Time, fallback string, reused bool) *DeliveryRoute {
	fraction, basis, off := 1.0, routeETABasisOriginal, false
	if progress, ok := maps.ProjectOnPolyline(sess.points, origin); ok {
		fraction = progress.RemainingFraction()
		basis = routeETABasisProgress
		off = progress.OffsetMeters > s.options.OffRouteMeters
	}
	return &DeliveryRoute{
		EncodedPolyline:       sess.polyline,
		DistanceMeters:        int64(math.Round(float64(sess.distance) * fraction)),
		DurationSeconds:       sess.duration * fraction,
		StaticDurationSeconds: sess.static * fraction,
		TrafficDelaySeconds:   sess.trafficDelay,
		Provider:              sess.provider,
		DestinationType:       sess.destKind,
		GeneratedAt:           sess.calculated,
		ExpiresAt:             sess.calculated.Add(s.options.MaxRouteAge),
		FallbackReason:        fallback,
		RouteDistanceMeters:   sess.distance,
		RouteDurationSeconds:  sess.duration,
		ETACalculatedAt:       now.UTC(),
		RiderLocationAt:       originAt.UTC(),
		ETABasis:              basis,
		RefreshReason:         sess.reason,
		Reused:                reused,
		OffRoute:              off,
	}
}

func deliveryRouteDestination(order *models.ActiveOrder) (routeDestination, error) {
	status := strings.ToLower(strings.TrimSpace(order.DeliveryStatus))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(order.Status))
	}
	switch status {
	case models.DeliveryStatusDelivered, models.DeliveryStatusCancelled, "canceled":
		return routeDestination{}, &DeliveryRouteError{Code: RouteErrOrderNotFound}
	case models.DeliveryStatusPickedUp, models.DeliveryStatusOnTheWay, "out_for_delivery":
		return coordinateDestination("drop", order.DropLatitude, order.DropLongitude)
	default:
		return coordinateDestination("pickup", order.PickupLatitude, order.PickupLongitude)
	}
}

func coordinateDestination(kind string, lat, lng *float64) (routeDestination, error) {
	if lat == nil || lng == nil {
		return routeDestination{}, &DeliveryRouteError{Code: RouteErrInvalidCoordinates}
	}
	c := maps.Coordinate{Latitude: *lat, Longitude: *lng}
	if !c.Valid() {
		return routeDestination{}, &DeliveryRouteError{Code: RouteErrInvalidCoordinates}
	}
	return routeDestination{kind: kind, coordinate: c}, nil
}

func mapRouteProviderError(err error) error {
	var routeErr *maps.RouteError
	if !errors.As(err, &routeErr) {
		return &DeliveryRouteError{Code: RouteErrUpstream, Err: err}
	}
	switch routeErr.Code {
	case maps.ProviderDisabled:
		return &DeliveryRouteError{Code: RouteErrProviderDisabled, Err: err}
	case maps.NotConfigured:
		return &DeliveryRouteError{Code: RouteErrNotConfigured, Err: err}
	case maps.InvalidCoordinates, maps.InvalidRequest:
		return &DeliveryRouteError{Code: RouteErrInvalidCoordinates, Err: err}
	case maps.NoRoute:
		return &DeliveryRouteError{Code: RouteErrNoRoute, Err: err}
	case maps.RateLimited:
		return &DeliveryRouteError{Code: RouteErrRateLimited, Err: err}
	default:
		return &DeliveryRouteError{Code: RouteErrUpstream, Err: err}
	}
}

func errorCode(err error) string {
	var routeErr *DeliveryRouteError
	if errors.As(err, &routeErr) {
		return routeErr.Code
	}
	return RouteErrUpstream
}

func (s *DeliveryRouteService) forget(key string) {
	s.mu.Lock()
	delete(s.sessions, key)
	s.mu.Unlock()
}

// pruneLocked drops state that can no longer matter. Route data therefore
// does not outlive its hold limit by more than the prune interval.
func (s *DeliveryRouteService) pruneLocked(now time.Time) {
	if !s.lastPrune.IsZero() && now.Sub(s.lastPrune) < routePruneInterval && len(s.sessions) < routeMaxSessions {
		return
	}
	s.lastPrune = now
	for key, sess := range s.sessions {
		if now.Sub(sess.calculated) >= s.options.MaxRouteAge {
			delete(s.sessions, key)
		}
	}
	for len(s.sessions) >= routeMaxSessions {
		var oldestKey string
		var oldest time.Time
		for key, sess := range s.sessions {
			if oldestKey == "" || sess.calculated.Before(oldest) {
				oldestKey, oldest = key, sess.calculated
			}
		}
		delete(s.sessions, oldestKey)
	}
	for key, at := range s.attempts {
		if now.Sub(at) > 10*time.Minute {
			delete(s.attempts, key)
		}
	}
	for key, f := range s.failures {
		if now.Sub(f.at) > 10*time.Minute {
			delete(s.failures, key)
		}
	}
	for id, oc := range s.orderCalls {
		if now.Sub(oc.last) > 6*time.Hour {
			delete(s.orderCalls, id)
		}
	}
}

func sessionKey(riderID string, orderID int) string {
	return strconv.Itoa(orderID) + ":" + strings.TrimSpace(riderID)
}

func sampleKey(c maps.Coordinate) string {
	return fmt.Sprintf("%.5f,%.5f", c.Latitude, c.Longitude)
}

func sameCoordinate(a, b maps.Coordinate) bool {
	return math.Abs(a.Latitude-b.Latitude) < 1e-5 && math.Abs(a.Longitude-b.Longitude) < 1e-5
}

func (s *DeliveryRouteService) observe(riderID string, orderID int, destination, reason string, reused bool, err error) {
	s.observeCalls(riderID, orderID, destination, reason, reused, err, -1)
}

// observeCalls emits one privacy-safe event: identifiers, destination type,
// reason and outcome only. No coordinates, addresses, polylines or keys.
func (s *DeliveryRouteService) observeCalls(riderID string, orderID int, destination, reason string, reused bool, err error, orderCalls int) {
	fields := dispatchtrace.Fields{
		"rider_id":         riderID,
		"order_id":         orderID,
		"destination_type": destination,
		"success":          err == nil,
		"reused":           reused,
	}
	if reason != "" {
		fields["refresh_reason"] = reason
	}
	if orderCalls >= 0 {
		fields["order_route_calls"] = orderCalls
	}
	if err != nil {
		fields["error_code"] = errorCode(err)
	}
	dispatchtrace.Emit("delivery.route", fields)
}

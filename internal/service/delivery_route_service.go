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
	RouteErrUpstream           = "ROUTE_UPSTREAM_UNAVAILABLE"
)

type activeOrderResolver interface {
	GetActiveOrder(context.Context, string) (*models.ActiveOrder, error)
}

type DeliveryRouteOptions struct {
	TrafficAware bool
	ResultTTL    time.Duration
	MinInterval  time.Duration
}

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

type routeCall struct {
	done   chan struct{}
	result *DeliveryRoute
	err    error
}

type DeliveryRouteService struct {
	active      activeOrderResolver
	provider    maps.RouteProvider
	options     DeliveryRouteOptions
	mu          sync.Mutex
	inflight    map[string]*routeCall
	lastRequest map[string]time.Time
	now         func() time.Time
}

func NewDeliveryRouteService(active activeOrderResolver, provider maps.RouteProvider, options DeliveryRouteOptions) *DeliveryRouteService {
	if options.ResultTTL <= 0 || options.ResultTTL > 5*time.Minute {
		options.ResultTTL = time.Minute
	}
	if options.MinInterval <= 0 {
		options.MinInterval = 5 * time.Second
	}
	return &DeliveryRouteService{
		active:      active,
		provider:    provider,
		options:     options,
		inflight:    make(map[string]*routeCall),
		lastRequest: make(map[string]time.Time),
		now:         time.Now,
	}
}

func (s *DeliveryRouteService) GetDeliveryRoute(ctx context.Context, riderID string, orderID int, orderType string, origin maps.Coordinate) (*DeliveryRoute, error) {
	if s == nil || s.active == nil || s.provider == nil {
		return nil, &DeliveryRouteError{Code: RouteErrProviderDisabled}
	}
	if strings.TrimSpace(riderID) == "" || orderID <= 0 {
		return nil, &DeliveryRouteError{Code: RouteErrOrderNotFound}
	}
	if !origin.Valid() {
		return nil, &DeliveryRouteError{Code: RouteErrInvalidCoordinates}
	}
	active, err := s.active.GetActiveOrder(ctx, riderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &DeliveryRouteError{Code: RouteErrOrderNotFound, Err: err}
		}
		return nil, &DeliveryRouteError{Code: RouteErrUpstream, Err: err}
	}
	if active == nil || active.OrderID != orderID {
		return nil, &DeliveryRouteError{Code: RouteErrOrderNotFound}
	}
	destination, err := deliveryRouteDestination(active)
	if err != nil {
		return nil, err
	}
	key := s.inflightKey(riderID, orderID, orderType, origin, destination)
	if call := s.findInflight(key); call != nil {
		<-call.done
		return call.result, call.err
	}
	rateKey := riderID + ":" + strconv.Itoa(orderID) + ":" + destination.kind
	if err := s.allow(rateKey); err != nil {
		return nil, err
	}
	call := &routeCall{done: make(chan struct{})}
	if existing := s.storeInflight(key, call); existing != nil {
		<-existing.done
		return existing.result, existing.err
	}
	call.result, call.err = s.compute(ctx, origin, destination)
	s.completeInflight(key, call)
	s.observe(riderID, orderID, destination.kind, call.err)
	return call.result, call.err
}

func (s *DeliveryRouteService) compute(ctx context.Context, origin maps.Coordinate, destination routeDestination) (*DeliveryRoute, error) {
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
	generated := result.CalculatedAt
	if generated.IsZero() {
		generated = s.now().UTC()
	}
	var delay *float64
	if result.StaticDurationSeconds > 0 && result.DurationSeconds > result.StaticDurationSeconds {
		value := result.DurationSeconds - result.StaticDurationSeconds
		delay = &value
	}
	return &DeliveryRoute{
		EncodedPolyline:       result.Polyline,
		DistanceMeters:        result.DistanceMeters,
		DurationSeconds:       result.DurationSeconds,
		StaticDurationSeconds: result.StaticDurationSeconds,
		TrafficDelaySeconds:   delay,
		Provider:              result.Provider,
		DestinationType:       destination.kind,
		GeneratedAt:           generated,
		ExpiresAt:             generated.Add(s.options.ResultTTL),
	}, nil
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

func (s *DeliveryRouteService) findInflight(key string) *routeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inflight[key]
}

func (s *DeliveryRouteService) storeInflight(key string, call *routeCall) *routeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.inflight[key]; existing != nil {
		return existing
	}
	s.inflight[key] = call
	return nil
}

func (s *DeliveryRouteService) completeInflight(key string, call *routeCall) {
	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()
	close(call.done)
}

func (s *DeliveryRouteService) allow(key string) error {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if last, ok := s.lastRequest[key]; ok && now.Sub(last) < s.options.MinInterval {
		return &DeliveryRouteError{Code: RouteErrRateLimited}
	}
	s.lastRequest[key] = now
	return nil
}

func (s *DeliveryRouteService) inflightKey(riderID string, orderID int, orderType string, origin maps.Coordinate, destination routeDestination) string {
	round := func(v float64) float64 {
		v = math.Round(v*1e5) / 1e5
		if v == 0 {
			return 0
		}
		return v
	}
	return fmt.Sprintf("%s:%d:%s:%.5f,%.5f:%.5f,%.5f:%s:%t",
		riderID, orderID, strings.ToLower(strings.TrimSpace(orderType)),
		round(origin.Latitude), round(origin.Longitude),
		round(destination.coordinate.Latitude), round(destination.coordinate.Longitude),
		destination.kind, s.options.TrafficAware)
}

func (s *DeliveryRouteService) observe(riderID string, orderID int, destination string, err error) {
	fields := dispatchtrace.Fields{
		"rider_id":         riderID,
		"order_id":         orderID,
		"destination_type": destination,
		"success":          err == nil,
	}
	var routeErr *DeliveryRouteError
	if errors.As(err, &routeErr) {
		fields["error_code"] = routeErr.Code
	}
	dispatchtrace.Emit("delivery.route", fields)
}

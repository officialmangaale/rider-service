package maps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/config"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
)

const maxResponseBytes = 2 << 20

type Observation struct {
	Operation, CorrelationID, Cache string
	Latency                         time.Duration
	Attempts, StatusCode            int
	ErrorCode                       ErrorCode
}
type Observer func(context.Context, Observation)
type correlationKey struct{}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

func logObservation(_ context.Context, o Observation) {
	dispatchtrace.Emit("maps.request", dispatchtrace.Fields{
		"provider": "google", "operation": o.Operation, "correlation_id": o.CorrelationID,
		"success": o.ErrorCode == "", "error_code": string(o.ErrorCode),
		"latency_ms": o.Latency.Milliseconds(), "status_code": o.StatusCode,
		"timeout": o.ErrorCode == Timeout, "attempts": o.Attempts, "cache": o.Cache,
	})
}

type GoogleRoutes struct {
	cfg     config.GoogleMaps
	client  *http.Client
	cache   RouteCache
	observe Observer
}

// NewGoogleRoutes performs no network I/O. Disabled and malformed configuration
// is reported by Capability and checked again before every operation/cache read.
func NewGoogleRoutes(cfg config.GoogleMaps, client *http.Client, cache RouteCache, observer Observer) *GoogleRoutes {
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	copyClient.Timeout = cfg.RequestTimeout
	// Never forward the API key to a redirected host.
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if observer == nil {
		observer = logObservation
	}
	return &GoogleRoutes{cfg: cfg, client: &copyClient, cache: cache, observe: observer}
}

func (g *GoogleRoutes) Capability() string { return g.cfg.RoutesCapability() }
func (g *GoogleRoutes) ready() error {
	if c := g.Capability(); c != "AVAILABLE" {
		return &RouteError{Code: ErrorCode(c)}
	}
	return nil
}
func (g *GoogleRoutes) finish(ctx context.Context, started time.Time, o *Observation, err error) {
	o.Latency = time.Since(started)
	o.CorrelationID, _ = ctx.Value(correlationKey{}).(string)
	var providerErr *RouteError
	if errors.As(err, &providerErr) {
		o.ErrorCode = providerErr.Code
		if providerErr.StatusCode != 0 {
			o.StatusCode = providerErr.StatusCode
		}
	}
	g.observe(ctx, *o)
}

func (g *GoogleRoutes) GetRoute(ctx context.Context, r RouteRequest) (result RouteResult, err error) {
	started := time.Now()
	o := Observation{Operation: "routes", Cache: "disabled"}
	defer func() { g.finish(ctx, started, &o, err) }()
	if err = g.ready(); err != nil {
		return
	}
	if !r.Origin.Valid() || !r.Destination.Valid() {
		return result, &RouteError{Code: InvalidCoordinates}
	}
	r.Options, err = r.Options.normalized()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, g.cfg.RequestTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return result, contextError(ctx.Err())
	}
	key := RouteCacheKey(r)
	if g.cache != nil && g.cfg.RouteCacheTTL > 0 {
		o.Cache = "miss"
		cacheCtx, cacheCancel := context.WithTimeout(ctx, 50*time.Millisecond)
		cached, hit, cacheErr := g.cache.Get(cacheCtx, key)
		cacheCancel()
		if cacheErr != nil {
			o.Cache = "error"
		}
		age := time.Since(cached.CalculatedAt)
		if hit && cacheErr == nil && age >= 0 && age < g.cfg.RouteCacheTTL && validCachedRoute(cached, r.Options) {
			o.Cache = "hit"
			return cached, nil
		}
	}
	body := map[string]any{"origin": waypoint(r.Origin), "destination": waypoint(r.Destination), "travelMode": r.Options.TravelMode}
	if r.Options.RoutingPreference != "" {
		body["routingPreference"] = r.Options.RoutingPreference
	}
	mask := "routes.distanceMeters,routes.duration,routes.staticDuration,routes.legs.distanceMeters,routes.legs.duration,routes.legs.staticDuration"
	if r.Options.IncludePolyline {
		mask += ",routes.polyline.encodedPolyline"
	}
	data, err := g.post(ctx, "/directions/v2:computeRoutes", mask, body, &o)
	if err != nil {
		return result, err
	}
	var payload struct {
		Routes []googleRoute `json:"routes"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return result, &RouteError{Code: InvalidResponse}
	}
	if len(payload.Routes) == 0 {
		return result, &RouteError{Code: NoRoute}
	}
	result, err = decodeRoute(payload.Routes[0], r.Options, time.Now().UTC())
	if err != nil {
		return result, err
	}
	if len(result.Legs) == 0 {
		return RouteResult{}, &RouteError{Code: InvalidResponse}
	}
	if g.cache != nil && g.cfg.RouteCacheTTL > 0 {
		cacheCtx, cacheCancel := context.WithTimeout(ctx, 50*time.Millisecond)
		if g.cache.Set(cacheCtx, key, result, g.cfg.RouteCacheTTL) != nil {
			o.Cache = "error"
		}
		cacheCancel()
	}
	return result, nil
}

func validCachedRoute(r RouteResult, options RouteOptions) bool {
	validLeg := func(l RouteLeg) bool {
		return l.DistanceMeters >= 0 && l.DurationSeconds >= 0 && l.StaticDurationSeconds >= 0 && !math.IsNaN(l.DurationSeconds) && !math.IsInf(l.DurationSeconds, 0) && !math.IsNaN(l.StaticDurationSeconds) && !math.IsInf(l.StaticDurationSeconds, 0)
	}
	if r.Provider != "google" || !validLeg(r.RouteLeg) || len(r.Legs) == 0 || (options.IncludePolyline && r.Polyline == "") {
		return false
	}
	for _, leg := range r.Legs {
		if !validLeg(leg) {
			return false
		}
	}
	return true
}

func (g *GoogleRoutes) GetRouteMatrix(ctx context.Context, r RouteMatrixRequest) (result RouteMatrixResult, err error) {
	started := time.Now()
	o := Observation{Operation: "matrix", Cache: "disabled"}
	defer func() { g.finish(ctx, started, &o, err) }()
	if err = g.ready(); err != nil {
		return
	}
	// A conservative bound fits all supported traffic preferences and caps cost.
	if len(r.Origins) == 0 || len(r.Destinations) == 0 || len(r.Origins) > 100 || len(r.Destinations) > 100 || len(r.Origins)*len(r.Destinations) > 100 {
		return result, &RouteError{Code: InvalidRequest}
	}
	origins, destinations := make([]any, len(r.Origins)), make([]any, len(r.Destinations))
	for i, c := range r.Origins {
		if !c.Valid() {
			return result, &RouteError{Code: InvalidCoordinates}
		}
		origins[i] = map[string]any{"waypoint": waypoint(c)}
	}
	for i, c := range r.Destinations {
		if !c.Valid() {
			return result, &RouteError{Code: InvalidCoordinates}
		}
		destinations[i] = map[string]any{"waypoint": waypoint(c)}
	}
	r.Options, err = r.Options.normalized()
	if err != nil {
		return
	}
	if r.Options.IncludePolyline {
		return result, &RouteError{Code: InvalidRequest}
	}
	ctx, cancel := context.WithTimeout(ctx, g.cfg.RequestTimeout)
	defer cancel()
	body := map[string]any{"origins": origins, "destinations": destinations, "travelMode": r.Options.TravelMode}
	if r.Options.RoutingPreference != "" {
		body["routingPreference"] = r.Options.RoutingPreference
	}
	data, err := g.post(ctx, "/distanceMatrix/v2:computeRouteMatrix", "originIndex,destinationIndex,status,condition,distanceMeters,duration,staticDuration", body, &o)
	if err != nil {
		return result, err
	}
	var elements []struct {
		OriginIndex      int `json:"originIndex"`
		DestinationIndex int `json:"destinationIndex"`
		Status           *struct {
			Code int `json:"code"`
		} `json:"status"`
		Condition string `json:"condition"`
		googleRoute
	}
	if json.Unmarshal(data, &elements) != nil || len(elements) != len(r.Origins)*len(r.Destinations) {
		return result, &RouteError{Code: InvalidResponse}
	}
	result.Provider, result.CalculatedAt = "google", time.Now().UTC()
	seen := make(map[[2]int]bool)
	for _, e := range elements {
		index := [2]int{e.OriginIndex, e.DestinationIndex}
		if e.OriginIndex < 0 || e.OriginIndex >= len(r.Origins) || e.DestinationIndex < 0 || e.DestinationIndex >= len(r.Destinations) || seen[index] || e.Status == nil {
			return RouteMatrixResult{}, &RouteError{Code: InvalidResponse}
		}
		seen[index] = true
		item := RouteMatrixElement{OriginIndex: e.OriginIndex, DestinationIndex: e.DestinationIndex}
		switch {
		case e.Status.Code != 0:
			code := UpstreamError
			switch e.Status.Code {
			case 3:
				code = InvalidRequest
			case 4:
				code = Timeout
			case 7, 16:
				code = NotConfigured
			case 8:
				code = RateLimited
			}
			item.Error = &RouteError{Code: code}
		case e.Condition == "ROUTE_NOT_FOUND":
			item.Error = &RouteError{Code: NoRoute}
		case e.Condition == "ROUTE_EXISTS":
			decoded, decodeErr := decodeRoute(e.googleRoute, r.Options, result.CalculatedAt)
			if decodeErr != nil {
				return RouteMatrixResult{}, decodeErr
			}
			item.Route = &decoded
		default:
			return RouteMatrixResult{}, &RouteError{Code: InvalidResponse}
		}
		result.Elements = append(result.Elements, item)
	}
	return result, nil
}

func waypoint(c Coordinate) any { return map[string]any{"location": map[string]any{"latLng": c}} }

type googleRoute struct {
	DistanceMeters int64  `json:"distanceMeters"`
	Duration       string `json:"duration"`
	StaticDuration string `json:"staticDuration"`
	Polyline       struct {
		Encoded string `json:"encodedPolyline"`
	} `json:"polyline"`
	Legs []googleRoute `json:"legs"`
}

func decodeRoute(r googleRoute, options RouteOptions, at time.Time) (RouteResult, error) {
	parseDuration := func(s string) (float64, bool) {
		if !strings.HasSuffix(s, "s") {
			return 0, false
		}
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "s"), 64)
		return n, err == nil && n >= 0 && !math.IsNaN(n) && !math.IsInf(n, 0)
	}
	duration, ok := parseDuration(r.Duration)
	static, staticOK := parseDuration(r.StaticDuration)
	if !ok || !staticOK || r.DistanceMeters < 0 || (options.IncludePolyline && r.Polyline.Encoded == "") {
		return RouteResult{}, &RouteError{Code: InvalidResponse}
	}
	result := RouteResult{RouteLeg: RouteLeg{DistanceMeters: r.DistanceMeters, DurationSeconds: duration, StaticDurationSeconds: static}, Provider: "google", CalculatedAt: at, Polyline: r.Polyline.Encoded}
	if strings.HasPrefix(options.RoutingPreference, "TRAFFIC_AWARE") {
		result.TrafficDurationSeconds = &duration
	}
	for _, leg := range r.Legs {
		decoded, err := decodeRoute(leg, RouteOptions{}, at)
		if err != nil {
			return RouteResult{}, err
		}
		result.Legs = append(result.Legs, decoded.RouteLeg)
	}
	return result, nil
}

func contextError(err error) *RouteError {
	if errors.Is(err, context.Canceled) {
		return &RouteError{Code: Cancelled}
	}
	return &RouteError{Code: Timeout}
}

func (g *GoogleRoutes) post(ctx context.Context, path, mask string, body any, o *Observation) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, &RouteError{Code: InvalidRequest}
	}
	for attempt := 0; attempt <= g.cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, contextError(ctx.Err())
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(g.cfg.BaseURL, "/")+path, bytes.NewReader(data))
		if err != nil {
			return nil, &RouteError{Code: NotConfigured}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-Api-Key", g.cfg.ServerAPIKey)
		req.Header.Set("X-Goog-FieldMask", mask)
		o.Attempts++
		resp, err := g.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, contextError(ctx.Err())
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, &RouteError{Code: Timeout}
			}
			// Transport failures are not retried: execution may already be billable.
			return nil, &RouteError{Code: UpstreamError}
		}
		o.StatusCode = resp.StatusCode
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		resp.Body.Close()
		if ctx.Err() != nil {
			return nil, contextError(ctx.Err())
		}
		if readErr != nil || len(raw) > maxResponseBytes {
			return nil, &RouteError{Code: InvalidResponse, StatusCode: resp.StatusCode}
		}
		if resp.StatusCode == http.StatusOK {
			return raw, nil
		}
		code := UpstreamError
		switch resp.StatusCode {
		case 400, 422:
			code = InvalidRequest
		case 401, 403:
			code = NotConfigured
		case 429:
			code = RateLimited
		}
		transient := resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 || resp.StatusCode == 500
		failure := &RouteError{Code: code, StatusCode: resp.StatusCode}
		if !transient || attempt == g.cfg.MaxRetries {
			return nil, failure
		}
		delay := time.Duration(100*(1<<attempt)+rand.IntN(100)) * time.Millisecond
		if header := resp.Header.Get("Retry-After"); header != "" {
			if seconds, parseErr := strconv.Atoi(header); parseErr == nil && seconds >= 0 {
				if seconds > 1 {
					return nil, failure
				}
				delay = max(delay, time.Duration(seconds)*time.Second)
			} else if date, parseErr := http.ParseTime(header); parseErr == nil {
				delay = max(delay, time.Until(date))
				if delay > time.Second {
					return nil, failure
				}
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, contextError(ctx.Err())
		case <-timer.C:
		}
	}
	return nil, &RouteError{Code: UpstreamError}
}

var _ RouteProvider = (*GoogleRoutes)(nil)

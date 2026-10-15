package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type GoogleMaps struct {
	RoutesEnabled        bool
	RoutesTrafficEnabled bool
	ServerAPIKey         string `json:"-"`
	BaseURL              string
	RequestTimeout       time.Duration
	MaxRetries           int
	// RouteCacheTTL bounds the provider-level (Redis) response cache. It is a
	// short performance cache, not the order-route reuse window.
	RouteCacheTTL time.Duration

	// Order-route reuse. A route is calculated when an order enters a delivery
	// phase and then reused for that order, rider and destination until the
	// rider materially leaves it, the destination changes, or it reaches
	// RouteMaxAge. Age alone below RouteMaxAge never triggers a provider call.
	RouteMaxAge           time.Duration
	OffRouteMeters        int
	OffRouteSamples       int
	DeviationCooldown     time.Duration
	FailureCooldown       time.Duration
	RouteMinInterval      time.Duration
	MaxCallsPerOrder      int
	MaxCallsPerMinute     int
	DailyBudget           int
	MaxConcurrentRequests int
}

// String prevents accidental credential disclosure through standard formatting.
func (c GoogleMaps) String() string {
	return fmt.Sprintf("GoogleMaps{routes=%t traffic=%t capability=%s}", c.RoutesEnabled, c.RoutesTrafficEnabled, c.RoutesCapability())
}
func (c GoogleMaps) GoString() string { return c.String() }

func boundedInt(key string, fallback, min, max int) int {
	v := getEnvInt(key, fallback)
	if v < min || v > max {
		return fallback
	}
	return v
}

func loadGoogleMaps() GoogleMaps {
	timeout := getEnvInt("GOOGLE_MAPS_TIMEOUT_MS", 5000)
	if timeout < 100 || timeout > 30000 {
		timeout = 5000
	}
	retries := getEnvInt("GOOGLE_MAPS_MAX_RETRIES", 1)
	if retries < 0 || retries > 2 {
		retries = 1
	}
	ttl := getEnvInt("GOOGLE_ROUTES_CACHE_TTL_SECONDS", 60)
	if ttl < 0 || ttl > 300 {
		ttl = 60
	}
	return GoogleMaps{
		RoutesEnabled: getEnvBool("GOOGLE_ROUTES_ENABLED", false),
		// Traffic-aware routing is billed as Routes Pro. Off unless explicitly
		// enabled; the default request is DRIVE + TRAFFIC_UNAWARE (Essentials).
		RoutesTrafficEnabled: getEnvBool("GOOGLE_ROUTES_TRAFFIC_ENABLED", false),
		ServerAPIKey:         strings.TrimSpace(os.Getenv("GOOGLE_MAPS_SERVER_API_KEY")),
		BaseURL:              strings.TrimRight(getEnv("GOOGLE_ROUTES_BASE_URL", "https://routes.googleapis.com"), "/"),
		RequestTimeout:       time.Duration(timeout) * time.Millisecond,
		MaxRetries:           retries,
		RouteCacheTTL:        time.Duration(ttl) * time.Second,

		// Hold limit: Google's terms do not grant long-lived route storage, so
		// this is short, capped at 30 minutes, and the data is in memory only.
		RouteMaxAge:           time.Duration(boundedInt("GOOGLE_ROUTES_REUSE_MAX_AGE_SECONDS", 900, 60, 1800)) * time.Second,
		OffRouteMeters:        boundedInt("GOOGLE_ROUTES_OFF_ROUTE_METERS", 100, 50, 500),
		OffRouteSamples:       boundedInt("GOOGLE_ROUTES_OFF_ROUTE_SAMPLES", 2, 1, 5),
		DeviationCooldown:     time.Duration(boundedInt("GOOGLE_ROUTES_DEVIATION_COOLDOWN_SECONDS", 45, 5, 600)) * time.Second,
		FailureCooldown:       time.Duration(boundedInt("GOOGLE_ROUTES_FAILURE_COOLDOWN_SECONDS", 30, 5, 600)) * time.Second,
		RouteMinInterval:      time.Duration(boundedInt("GOOGLE_ROUTES_MIN_INTERVAL_SECONDS", 5, 1, 120)) * time.Second,
		MaxCallsPerOrder:      boundedInt("GOOGLE_ROUTES_MAX_CALLS_PER_ORDER", 8, 1, 100),
		MaxCallsPerMinute:     boundedInt("GOOGLE_ROUTES_MAX_CALLS_PER_MINUTE", 30, 1, 1000),
		DailyBudget:           boundedInt("GOOGLE_ROUTES_DAILY_BUDGET", 2000, 1, 1000000),
		MaxConcurrentRequests: boundedInt("GOOGLE_ROUTES_MAX_CONCURRENCY", 4, 1, 64),
	}
}

// RoutesCapability never prevents the rest of the service from starting.
func (c GoogleMaps) RoutesCapability() string {
	if !c.RoutesEnabled {
		return "PROVIDER_DISABLED"
	}
	if strings.TrimSpace(c.ServerAPIKey) == "" {
		return "NOT_CONFIGURED"
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "NOT_CONFIGURED"
	}
	// Plain HTTP is allowed only for loopback mock servers.
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return "NOT_CONFIGURED"
	}
	if c.RequestTimeout <= 0 || c.RequestTimeout > 30*time.Second || c.MaxRetries < 0 || c.MaxRetries > 2 || c.RouteCacheTTL < 0 || c.RouteCacheTTL > 5*time.Minute {
		return "NOT_CONFIGURED"
	}
	return "AVAILABLE"
}

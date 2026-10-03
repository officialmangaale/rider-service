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
	RouteCacheTTL        time.Duration
}

// String prevents accidental credential disclosure through standard formatting.
func (c GoogleMaps) String() string {
	return fmt.Sprintf("GoogleMaps{routes=%t capability=%s}", c.RoutesEnabled, c.RoutesCapability())
}
func (c GoogleMaps) GoString() string { return c.String() }

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
		RoutesEnabled:        getEnvBool("GOOGLE_ROUTES_ENABLED", false),
		RoutesTrafficEnabled: getEnvBool("GOOGLE_ROUTES_TRAFFIC_ENABLED", false),
		ServerAPIKey:         strings.TrimSpace(os.Getenv("GOOGLE_MAPS_SERVER_API_KEY")),
		BaseURL:              strings.TrimRight(getEnv("GOOGLE_ROUTES_BASE_URL", "https://routes.googleapis.com"), "/"),
		RequestTimeout:       time.Duration(timeout) * time.Millisecond,
		MaxRetries:           retries,
		RouteCacheTTL:        time.Duration(ttl) * time.Second,
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

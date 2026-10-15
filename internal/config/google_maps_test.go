package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGoogleMapsConfiguration(t *testing.T) {
	for _, test := range []struct{ flag, key, want string }{
		{"false", "", "PROVIDER_DISABLED"}, {"true", "", "NOT_CONFIGURED"},
		{"false", "test-only", "PROVIDER_DISABLED"}, {"true", "test-only", "AVAILABLE"},
	} {
		t.Run(test.flag+test.want, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://unused")
			t.Setenv("JWT_SECRET", "test")
			t.Setenv("GOOGLE_ROUTES_ENABLED", test.flag)
			t.Setenv("GOOGLE_MAPS_SERVER_API_KEY", test.key)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.GoogleMaps.RoutesCapability(); got != test.want {
				t.Fatalf("got %s want %s", got, test.want)
			}
			encoded, _ := json.Marshal(cfg.GoogleMaps)
			if test.key != "" && (strings.Contains(string(encoded), test.key) || strings.Contains(fmt.Sprintf("%+v %#v", cfg.GoogleMaps, cfg.GoogleMaps), test.key)) {
				t.Fatal("credential leaked in formatting")
			}
		})
	}
}

func TestGoogleMapsConfigurationRejectsUnsafeURL(t *testing.T) {
	t.Setenv("GOOGLE_ROUTES_ENABLED", "true")
	t.Setenv("GOOGLE_MAPS_SERVER_API_KEY", "test-only")
	for _, raw := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?key=test", "https://example.com/path"} {
		t.Setenv("GOOGLE_ROUTES_BASE_URL", raw)
		if c := loadGoogleMaps(); c.RoutesCapability() != "NOT_CONFIGURED" {
			t.Fatalf("accepted unsafe base URL")
		}
	}
}

func TestGoogleMapsDefaultsAreCostSafe(t *testing.T) {
	for _, key := range []string{
		"GOOGLE_ROUTES_ENABLED", "GOOGLE_ROUTES_TRAFFIC_ENABLED", "GOOGLE_ROUTES_REUSE_MAX_AGE_SECONDS",
		"GOOGLE_ROUTES_OFF_ROUTE_METERS", "GOOGLE_ROUTES_MAX_CALLS_PER_ORDER", "GOOGLE_ROUTES_MAX_CALLS_PER_MINUTE",
		"GOOGLE_ROUTES_DAILY_BUDGET", "GOOGLE_ROUTES_MAX_CONCURRENCY",
	} {
		t.Setenv(key, "")
	}
	c := loadGoogleMaps()
	if c.RoutesEnabled || c.RoutesTrafficEnabled {
		t.Fatalf("routes and traffic-aware routing must default off: %+v", c)
	}
	if c.RouteMaxAge != 15*time.Minute || c.OffRouteMeters != 100 || c.OffRouteSamples != 2 {
		t.Fatalf("unexpected reuse defaults: %+v", c)
	}
	if c.MaxCallsPerOrder != 8 || c.MaxCallsPerMinute != 30 || c.DailyBudget != 2000 || c.MaxConcurrentRequests != 4 {
		t.Fatalf("unexpected budget defaults: %+v", c)
	}
}

func TestGoogleMapsReuseSettingsAreBounded(t *testing.T) {
	// Out-of-range values fall back to the safe default rather than disabling
	// a protection (a 24h hold limit or an unlimited budget).
	t.Setenv("GOOGLE_ROUTES_REUSE_MAX_AGE_SECONDS", "86400")
	t.Setenv("GOOGLE_ROUTES_DAILY_BUDGET", "0")
	t.Setenv("GOOGLE_ROUTES_MAX_CALLS_PER_ORDER", "-3")
	c := loadGoogleMaps()
	if c.RouteMaxAge != 15*time.Minute || c.DailyBudget != 2000 || c.MaxCallsPerOrder != 8 {
		t.Fatalf("out-of-range values must fall back to defaults: %+v", c)
	}
	t.Setenv("GOOGLE_ROUTES_REUSE_MAX_AGE_SECONDS", "600")
	if got := loadGoogleMaps().RouteMaxAge; got != 10*time.Minute {
		t.Fatalf("valid override ignored: %v", got)
	}
}

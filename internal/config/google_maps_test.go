package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
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

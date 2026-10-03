package maps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type memoryRouteCache struct {
	values map[string]RouteResult
	ttl    time.Duration
	fail   bool
	gets   int
}

func (c *memoryRouteCache) Get(_ context.Context, key string) (RouteResult, bool, error) {
	c.gets++
	if c.fail {
		return RouteResult{}, false, errors.New("offline")
	}
	v, ok := c.values[key]
	return v, ok, nil
}
func (c *memoryRouteCache) Set(_ context.Context, key string, v RouteResult, ttl time.Duration) error {
	c.ttl = ttl
	c.values[key] = v
	return nil
}

func TestCacheKey(t *testing.T) {
	r := testRequest()
	key := RouteCacheKey(r)
	r.Origin.Latitude += 0.0000001
	if RouteCacheKey(r) != key {
		t.Fatal("normalization failed")
	}
	r.Options.TravelMode = "DRIVE"
	r.Options.RoutingPreference = "TRAFFIC_UNAWARE"
	if RouteCacheKey(r) != key {
		t.Fatal("default normalization failed")
	}
	r.Options.IncludePolyline = true
	if RouteCacheKey(r) == key {
		t.Fatal("polyline collision")
	}
	r.Options.IncludePolyline = false
	r.Options.RoutingPreference = "TRAFFIC_AWARE"
	if RouteCacheKey(r) == key {
		t.Fatal("traffic collision")
	}
}

func TestCacheHitExpiryFailureAndDisable(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, routeJSON) }))
	defer server.Close()
	cache := &memoryRouteCache{values: make(map[string]RouteResult)}
	cfg := testConfig(server.URL)
	cfg.RouteCacheTTL = time.Minute
	client := NewGoogleRoutes(cfg, nil, cache, quiet)
	for i := 0; i < 2; i++ {
		if _, err := client.GetRoute(context.Background(), testRequest()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || cache.ttl != time.Minute {
		t.Fatal("cache not used")
	}
	key := RouteCacheKey(testRequest())
	value := cache.values[key]
	value.CalculatedAt = time.Now().Add(-2 * time.Minute)
	cache.values[key] = value
	if _, err := client.GetRoute(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("stale route served")
	}
	cache.fail = true
	if _, err := client.GetRoute(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("cache failure did not fall back")
	}
	gets := cache.gets
	cfg.RoutesEnabled = false
	_, err := NewGoogleRoutes(cfg, nil, cache, quiet).GetRoute(context.Background(), testRequest())
	requireCode(t, err, ProviderDisabled)
	if cache.gets != gets || calls != 3 {
		t.Fatal("disabled capability used cache/network")
	}
	cfg.RoutesEnabled = true
	cfg.RouteCacheTTL = 0
	if _, err := NewGoogleRoutes(cfg, nil, cache, quiet).GetRoute(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if cache.gets != gets {
		t.Fatal("zero TTL accessed cache")
	}
}

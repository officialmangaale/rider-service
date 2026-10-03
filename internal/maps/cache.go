package maps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RouteCache interface {
	Get(context.Context, string) (RouteResult, bool, error)
	Set(context.Context, string, RouteResult, time.Duration) error
}

// Coordinates are normalized to about one metre. Version/provider/options are
// included; correlation IDs and credentials never form part of the key.
func RouteCacheKey(r RouteRequest) string {
	o, _ := r.Options.normalized()
	round := func(v float64) float64 {
		v = math.Round(v*1e5) / 1e5
		if v == 0 {
			return 0
		}
		return v
	}
	value := fmt.Sprintf("%.5f,%.5f:%.5f,%.5f:%s:%s:%t", round(r.Origin.Latitude), round(r.Origin.Longitude), round(r.Destination.Latitude), round(r.Destination.Longitude), o.TravelMode, o.RoutingPreference, o.IncludePolyline)
	sum := sha256.Sum256([]byte(value))
	return "maps:google:route:v1:" + hex.EncodeToString(sum[:])
}

// Inject an existing Redis client. This adapter never connects during startup.
type RedisRouteCache struct{ Client redis.UniversalClient }

func NewRedisRouteCache(redisURL string) (RouteCache, io.Closer, error) {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil, nil, nil
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		opt = &redis.Options{Addr: redisURL}
	}
	client := redis.NewClient(opt)
	return RedisRouteCache{Client: client}, client, nil
}

func (c RedisRouteCache) Get(ctx context.Context, key string) (RouteResult, bool, error) {
	data, err := c.Client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return RouteResult{}, false, nil
	}
	if err != nil {
		return RouteResult{}, false, err
	}
	var result RouteResult
	if len(data) > maxResponseBytes {
		return result, false, &RouteError{Code: InvalidResponse}
	}
	err = json.Unmarshal(data, &result)
	return result, err == nil, err
}
func (c RedisRouteCache) Set(ctx context.Context, key string, result RouteResult, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.Client.Set(ctx, key, data, ttl).Err()
}

package cache

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/redis/go-redis/v9"
)

const (
	AvailableRidersKey = "riders:available"
	RiderLocationKey   = "riders:location"
	locationUpdatedKey = "riders:location:updated_at"
)

// RedisDispatchCache is an optional fast path for dispatch search and locks.
// PostgreSQL remains the durable ledger; Redis accelerates fan-out and protects
// the first-accept race across service instances.
type RedisDispatchCache struct {
	client *redis.Client
}

func NewRedisDispatchCache(redisURL string) (*RedisDispatchCache, error) {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil, nil
	}

	var opt *redis.Options
	var err error
	if strings.Contains(redisURL, "://") {
		opt, err = redis.ParseURL(redisURL)
	} else {
		opt = &redis.Options{Addr: redisURL}
	}
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &RedisDispatchCache{client: redis.NewClient(opt)}, nil
}

func (c *RedisDispatchCache) Enabled() bool {
	return c != nil && c.client != nil
}

func (c *RedisDispatchCache) Close() error {
	if !c.Enabled() {
		return nil
	}
	return c.client.Close()
}

func (c *RedisDispatchCache) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	return c.client.Ping(ctx).Err()
}

func (c *RedisDispatchCache) UpdateRiderLocation(ctx context.Context, riderID string, lat, lng float64) error {
	if !c.Enabled() || strings.TrimSpace(riderID) == "" {
		return nil
	}
	now := time.Now().UTC().Unix()
	pipe := c.client.TxPipeline()
	pipe.GeoAdd(ctx, RiderLocationKey, &redis.GeoLocation{
		Name:      strings.TrimSpace(riderID),
		Latitude:  lat,
		Longitude: lng,
	})
	pipe.HSet(ctx, locationUpdatedKey, riderID, now)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisDispatchCache) UpdateRiderAvailability(ctx context.Context, riderID string, isOnline, isAvailable bool, currentOrderID *int) error {
	if !c.Enabled() || strings.TrimSpace(riderID) == "" {
		return nil
	}
	riderID = strings.TrimSpace(riderID)
	availabilityKey := riderAvailabilityKey(riderID)
	currentOrder := ""
	if currentOrderID != nil && *currentOrderID > 0 {
		currentOrder = fmt.Sprintf("%d", *currentOrderID)
	}

	pipe := c.client.TxPipeline()
	if isOnline && isAvailable && currentOrder == "" {
		pipe.SAdd(ctx, AvailableRidersKey, riderID)
	} else {
		pipe.SRem(ctx, AvailableRidersKey, riderID)
	}
	pipe.HSet(ctx, availabilityKey, map[string]interface{}{
		"is_online":        boolString(isOnline),
		"is_available":     boolString(isAvailable),
		"current_order_id": currentOrder,
		"updated_at":       time.Now().UTC().Unix(),
	})
	_, err := pipe.Exec(ctx)
	return err
}

func (c *RedisDispatchCache) FindNearestRiders(ctx context.Context, pickupLat, pickupLng, radiusKm float64, maxRiders int) ([]models.NearbyRider, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if maxRiders <= 0 {
		maxRiders = 5
	}

	locations, err := c.client.GeoSearchLocation(ctx, RiderLocationKey, &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  pickupLng,
			Latitude:   pickupLat,
			Radius:     radiusKm,
			RadiusUnit: "km",
			Sort:       "ASC",
			Count:      maxRiders * 3,
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Unix()
	out := make([]models.NearbyRider, 0, maxRiders)
	for _, loc := range locations {
		riderID := strings.TrimSpace(loc.Name)
		if riderID == "" {
			continue
		}
		available, err := c.client.SIsMember(ctx, AvailableRidersKey, riderID).Result()
		if err != nil || !available {
			continue
		}
		updatedAt, err := c.client.HGet(ctx, locationUpdatedKey, riderID).Int64()
		if err != nil || now-updatedAt > int64((5*time.Minute).Seconds()) {
			continue
		}
		availability, err := c.client.HGetAll(ctx, riderAvailabilityKey(riderID)).Result()
		if err != nil {
			continue
		}
		if availability["is_online"] != "true" ||
			availability["is_available"] != "true" ||
			strings.TrimSpace(availability["current_order_id"]) != "" {
			continue
		}
		out = append(out, models.NearbyRider{
			RiderID:    riderID,
			Latitude:   loc.Latitude,
			Longitude:  loc.Longitude,
			DistanceKm: math.Round(loc.Dist*100) / 100,
		})
		if len(out) >= maxRiders {
			break
		}
	}
	return out, nil
}

func (c *RedisDispatchCache) SetPendingOrder(ctx context.Context, orderID int, ttl time.Duration) error {
	if !c.Enabled() || orderID <= 0 {
		return nil
	}
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return c.client.Set(ctx, pendingOrderKey(orderID), "1", ttl).Err()
}

func (c *RedisDispatchCache) ClearPendingOrder(ctx context.Context, orderID int) {
	if !c.Enabled() || orderID <= 0 {
		return
	}
	if err := c.client.Del(ctx, pendingOrderKey(orderID)).Err(); err != nil {
		log.Printf("[DELIVERY] Redis pending order clear failed order_id=%d err=%v", orderID, err)
	}
}

func (c *RedisDispatchCache) TryAcceptOrder(ctx context.Context, orderID int, riderID string, ttl time.Duration) (bool, error) {
	if !c.Enabled() || orderID <= 0 || strings.TrimSpace(riderID) == "" {
		return true, nil
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return c.client.SetNX(ctx, orderLockKey(orderID), strings.TrimSpace(riderID), ttl).Result()
}

func (c *RedisDispatchCache) ReleaseOrderLock(ctx context.Context, orderID int, riderID string) {
	if !c.Enabled() || orderID <= 0 || strings.TrimSpace(riderID) == "" {
		return
	}
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`
	if err := c.client.Eval(ctx, script, []string{orderLockKey(orderID)}, strings.TrimSpace(riderID)).Err(); err != nil {
		log.Printf("[DELIVERY] Redis order lock release failed order_id=%d rider_id=%s err=%v", orderID, riderID, err)
	}
}

func (c *RedisDispatchCache) MarkOrderAssigned(ctx context.Context, orderID int) {
	c.ClearPendingOrder(ctx, orderID)
}

func riderAvailabilityKey(riderID string) string {
	return "rider:availability:" + strings.TrimSpace(riderID)
}

func pendingOrderKey(orderID int) string {
	return fmt.Sprintf("order:pending:%d", orderID)
}

func orderLockKey(orderID int) string {
	return fmt.Sprintf("lock:order:%d", orderID)
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

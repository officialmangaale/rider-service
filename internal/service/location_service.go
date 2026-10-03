package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/geo"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

var ErrInvalidRiderLocation = errors.New("invalid rider location")

type RiderLocationUpdate struct {
	Latitude       float64
	Longitude      float64
	Heading        *float64
	Speed          *float64
	AccuracyMeters *float64
	RecordedAt     string
	Source         string
	AppState       string
	Sequence       *int64
	ReceivedAt     time.Time
}

// LocationService handles rider location tracking.
type LocationService struct {
	riderRepo   *repository.RiderRepository
	historyRepo *repository.LocationHistoryRepository
	indexer     RiderLocationIndexer
	notifier    RiderLocationNotifier
}

// RiderLocationIndexer receives a location after PostgreSQL has stored it,
// e.g. to update the Redis dispatch index. Implemented by DeliveryService.
type RiderLocationIndexer interface {
	IndexRiderLocation(ctx context.Context, riderID string, lat, lng float64)
}

// RiderLocationNotifier publishes a stored rider location to the active
// delivery tracking channel. Implemented by DeliveryService.
type RiderLocationNotifier interface {
	NotifyRiderLocationUpdated(ctx context.Context, riderID string, update RiderLocationUpdate)
}

// SetLocationIndexer wires the post-persistence index. Optional; nil keeps
// PostgreSQL-only behaviour.
func (s *LocationService) SetLocationIndexer(indexer RiderLocationIndexer) {
	s.indexer = indexer
}

func (s *LocationService) SetLocationNotifier(notifier RiderLocationNotifier) {
	s.notifier = notifier
}

// NewLocationService creates a new LocationService.
func NewLocationService(riderRepo *repository.RiderRepository, historyRepo *repository.LocationHistoryRepository) *LocationService {
	return &LocationService{riderRepo: riderRepo, historyRepo: historyRepo}
}

// UpdateLocation updates the rider's current position in users table and logs to history.
func (s *LocationService) UpdateLocation(ctx context.Context, userID string, lat, lng float64, heading, speed *float64) (*dto.UpdateLocationResponse, error) {
	return s.UpdateRiderLocation(ctx, userID, RiderLocationUpdate{
		Latitude:  lat,
		Longitude: lng,
		Heading:   heading,
		Speed:     speed,
	})
}

// UpdateRiderLocation is the canonical rider location update operation used by
// both public location routes. Keep persistence here so the endpoints cannot
// drift into different production behavior again.
func (s *LocationService) UpdateRiderLocation(ctx context.Context, userID string, update RiderLocationUpdate) (*dto.UpdateLocationResponse, error) {
	if err := validateRiderCoordinatePair(update.Latitude, update.Longitude); err != nil {
		return nil, err
	}
	if update.ReceivedAt.IsZero() {
		update.ReceivedAt = time.Now().UTC()
	}

	// Update current position in users table
	lastUpdate, isAvailable, err := s.riderRepo.UpdateLocation(ctx, userID, update.Latitude, update.Longitude)
	if err != nil {
		return nil, err
	}
	// rider_locations is what dispatch reads (FindNearestRiders and the
	// own-rider liveness check). Its error used to be discarded, so the app
	// was told "updated" while dispatch still saw the old fix and treated the
	// rider as stale. Failing the request makes the app retry on its next tick.
	if err := s.riderRepo.UpsertRealtimeLocation(ctx, userID, update.Latitude, update.Longitude); err != nil {
		return nil, fmt.Errorf("update dispatch location: %w", err)
	}
	// Only after PostgreSQL, the source of truth, has the fix.
	if s.indexer != nil {
		s.indexer.IndexRiderLocation(ctx, userID, update.Latitude, update.Longitude)
	}
	// Log to location history (fire-and-forget for high frequency)
	_ = s.historyRepo.Record(ctx, userID, update.Latitude, update.Longitude, update.Heading, update.Speed)

	if s.notifier != nil {
		s.notifier.NotifyRiderLocationUpdated(ctx, userID, update)
	}

	if lastUpdate == nil {
		return &dto.UpdateLocationResponse{IsAvailable: isAvailable}, nil
	}

	return &dto.UpdateLocationResponse{
		IsAvailable:        isAvailable,
		LastLocationUpdate: *lastUpdate,
	}, nil
}

// GetCurrentLocation returns the rider's last known position from users table.
func (s *LocationService) GetCurrentLocation(ctx context.Context, userID string) (map[string]interface{}, error) {
	lat, lng, lastUpdate, err := s.riderRepo.GetCurrentLocation(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"latitude":             lat,
		"longitude":            lng,
		"last_location_update": lastUpdate,
	}, nil
}

func validateRiderCoordinatePair(lat, lng float64) error {
	if !geo.ValidCoordinate(lat, lng) {
		return ErrInvalidRiderLocation
	}
	return nil
}

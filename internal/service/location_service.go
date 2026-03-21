package service

import (
	"context"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// LocationService handles rider location tracking.
type LocationService struct {
	riderRepo   *repository.RiderRepository
	historyRepo *repository.LocationHistoryRepository
}

// NewLocationService creates a new LocationService.
func NewLocationService(riderRepo *repository.RiderRepository, historyRepo *repository.LocationHistoryRepository) *LocationService {
	return &LocationService{riderRepo: riderRepo, historyRepo: historyRepo}
}

// UpdateLocation updates the rider's current position in users table and logs to history.
func (s *LocationService) UpdateLocation(ctx context.Context, userID string, lat, lng float64, heading, speed *float64) (*dto.UpdateLocationResponse, error) {
	// Update current position in users table
	lastUpdate, isAvailable, err := s.riderRepo.UpdateLocation(ctx, userID, lat, lng)
	if err != nil {
		return nil, err
	}
	// Log to location history (fire-and-forget for high frequency)
	_ = s.historyRepo.Record(ctx, userID, lat, lng, heading, speed)

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

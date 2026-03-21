package service

import (
	"context"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// EarningsService handles rider earnings queries.
type EarningsService struct {
	earningsRepo *repository.EarningsRepository
}

// NewEarningsService creates a new EarningsService.
func NewEarningsService(earningsRepo *repository.EarningsRepository) *EarningsService {
	return &EarningsService{earningsRepo: earningsRepo}
}

// GetSummary returns aggregated earnings summary.
func (s *EarningsService) GetSummary(ctx context.Context, riderID string) (*models.EarningsSummary, error) {
	return s.earningsRepo.GetSummary(ctx, riderID)
}

// GetHistory returns paginated earnings history.
func (s *EarningsService) GetHistory(ctx context.Context, riderID string, limit, offset int) ([]*models.RiderEarning, int64, error) {
	return s.earningsRepo.GetHistory(ctx, riderID, limit, offset)
}

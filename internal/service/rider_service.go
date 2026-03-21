package service

import (
	"context"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// RiderService handles rider profile and availability logic.
type RiderService struct {
	riderRepo    *repository.RiderRepository
	orderRepo    *repository.OrderRepository
	earningsRepo *repository.EarningsRepository
}

// NewRiderService creates a new RiderService.
func NewRiderService(riderRepo *repository.RiderRepository, orderRepo *repository.OrderRepository, earningsRepo *repository.EarningsRepository) *RiderService {
	return &RiderService{riderRepo: riderRepo, orderRepo: orderRepo, earningsRepo: earningsRepo}
}

// GetProfile returns the rider's profile from the users table.
func (s *RiderService) GetProfile(ctx context.Context, userID string) (*models.User, error) {
	return s.riderRepo.GetByID(ctx, userID)
}

// UpdateProfile updates basic profile fields.
func (s *RiderService) UpdateProfile(ctx context.Context, userID string, firstName, lastName, email, displayName, avatarURL *string) (*models.User, error) {
	return s.riderRepo.UpdateProfile(ctx, userID, firstName, lastName, email, displayName, avatarURL)
}

// UpdateVehicle updates vehicle-related fields.
func (s *RiderService) UpdateVehicle(ctx context.Context, userID string, vehicleType, regNumber, vehicleDetails, insuranceDetails *string, maxCapacity *float64, licenseNumber, licenseExpiry *string) (*models.User, error) {
	return s.riderRepo.UpdateVehicle(ctx, userID, vehicleType, regNumber, vehicleDetails, insuranceDetails, maxCapacity, licenseNumber, licenseExpiry)
}

// UpdateBankDetails updates bank/payout info.
func (s *RiderService) UpdateBankDetails(ctx context.Context, userID string, bankDetails, payoutMethods *string) (*models.User, error) {
	return s.riderRepo.UpdateBankDetails(ctx, userID, bankDetails, payoutMethods)
}

// UpdateKYC updates KYC metadata.
func (s *RiderService) UpdateKYC(ctx context.Context, userID string, kycData, verificationDocs *string) (*models.User, error) {
	return s.riderRepo.UpdateKYC(ctx, userID, kycData, verificationDocs)
}

// GetOnboardingStatus checks profile completion.
func (s *RiderService) GetOnboardingStatus(ctx context.Context, userID string) (*models.OnboardingStatus, error) {
	rider, err := s.riderRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	status := &models.OnboardingStatus{
		ProfileComplete: rider.FirstName != nil && rider.Phone != nil,
		VehicleComplete: rider.VehicleType != nil && rider.VehicleRegistrationNumber != nil,
		KYCComplete:     rider.KYCVerified,
		BankComplete:    rider.BankDetails != nil,
	}
	status.FullyOnboarded = status.ProfileComplete && status.VehicleComplete && status.KYCComplete && status.BankComplete

	return status, nil
}

// GoOnline sets the rider as available.
func (s *RiderService) GoOnline(ctx context.Context, userID string) error {
	return s.riderRepo.SetAvailability(ctx, userID, true)
}

// GoOffline sets the rider as unavailable.
func (s *RiderService) GoOffline(ctx context.Context, userID string) error {
	return s.riderRepo.SetAvailability(ctx, userID, false)
}

// GetAvailability returns current availability and trip state.
func (s *RiderService) GetAvailability(ctx context.Context, userID string) (bool, bool, error) {
	return s.riderRepo.GetAvailability(ctx, userID)
}

// GetDashboard returns combined dashboard data.
func (s *RiderService) GetDashboard(ctx context.Context, userID string) (*models.DashboardData, error) {
	rider, err := s.riderRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	activeOrder, _ := s.orderRepo.GetActiveOrderForRider(ctx, userID)
	earnings, _ := s.earningsRepo.GetSummary(ctx, userID)

	return &models.DashboardData{
		Rider:       rider,
		ActiveOrder: activeOrder,
		Earnings:    earnings,
	}, nil
}

package service

import (
	"context"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
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
func (s *RiderService) UpdateKYC(ctx context.Context, userID string, licenseNumber, kycData, verificationDocs *string) (*models.User, error) {
	return s.riderRepo.UpdateKYC(ctx, userID, licenseNumber, kycData, verificationDocs)
}

// GetOnboardingStatus checks profile completion.
func (s *RiderService) GetOnboardingStatus(ctx context.Context, userID string) (*dto.OnboardingStatusResponse, error) {
	rider, err := s.riderRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	profileComplete := rider.FirstName != nil && rider.Phone != nil
	vehicleComplete := rider.VehicleType != nil && rider.VehicleRegistrationNumber != nil
	kycComplete := rider.KYCVerified || rider.VerificationDocs != nil
	bankComplete := rider.BankDetails != nil

	completed := []string{}
	pending := []string{}

	if profileComplete {
		completed = append(completed, "profile")
	} else {
		pending = append(pending, "profile")
	}

	if kycComplete {
		completed = append(completed, "kyc")
	} else {
		pending = append(pending, "kyc")
	}

	if vehicleComplete {
		completed = append(completed, "vehicle")
	} else {
		pending = append(pending, "vehicle")
	}

	if bankComplete {
		completed = append(completed, "bank_details")
	} else {
		pending = append(pending, "bank_details")
	}

	status := "pending_documents"
	isReady := false
	if len(pending) == 0 {
		if rider.KYCVerified {
			status = "approved"
			isReady = true
		} else {
			status = "under_review"
		}
	}

	return &dto.OnboardingStatusResponse{
		CurrentStatus:   status,
		CompletedSteps:  completed,
		PendingSteps:    pending,
		RejectionReason: nil,
		IsReadyToRide:   isReady,
	}, nil
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

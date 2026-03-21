package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// RiderHandler handles rider profile and availability endpoints.
type RiderHandler struct {
	riderSvc *service.RiderService
}

// NewRiderHandler creates a new RiderHandler.
func NewRiderHandler(riderSvc *service.RiderService) *RiderHandler {
	return &RiderHandler{riderSvc: riderSvc}
}

// GetProfile returns the rider's profile.
func (h *RiderHandler) GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	rider, err := h.riderSvc.GetProfile(c.Request.Context(), userID)
	if err != nil {
		dto.NotFound(c, "Rider profile not found")
		return
	}
	dto.Success(c, http.StatusOK, "profile fetched", rider)
}

// UpdateProfile updates basic profile fields.
func (h *RiderHandler) UpdateProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}

	rider, err := h.riderSvc.UpdateProfile(c.Request.Context(), userID, req.FirstName, req.LastName, req.Email, req.DisplayName, req.AvatarURL)
	if err != nil {
		dto.InternalError(c, "Failed to update profile")
		return
	}
	dto.Success(c, http.StatusOK, "profile updated", rider)
}

// UpdateVehicle updates vehicle details.
func (h *RiderHandler) UpdateVehicle(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req dto.UpdateVehicleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}

	rider, err := h.riderSvc.UpdateVehicle(c.Request.Context(), userID, req.VehicleType, req.VehicleRegistrationNumber, req.VehicleDetails, req.InsuranceDetails, req.MaxCarryCapacityKg, req.LicenseNumber, req.LicenseExpiry)
	if err != nil {
		dto.InternalError(c, "Failed to update vehicle")
		return
	}
	dto.Success(c, http.StatusOK, "vehicle updated", rider)
}

// UpdateBankDetails updates bank details.
func (h *RiderHandler) UpdateBankDetails(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req dto.UpdateBankDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}

	rider, err := h.riderSvc.UpdateBankDetails(c.Request.Context(), userID, req.BankDetails, req.PayoutMethods)
	if err != nil {
		dto.InternalError(c, "Failed to update bank details")
		return
	}
	dto.Success(c, http.StatusOK, "bank details updated", rider)
}

// UpdateKYC updates KYC data.
func (h *RiderHandler) UpdateKYC(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req dto.UpdateKYCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}

	rider, err := h.riderSvc.UpdateKYC(c.Request.Context(), userID, req.KYCData, req.VerificationDocs)
	if err != nil {
		dto.InternalError(c, "Failed to update KYC")
		return
	}
	dto.Success(c, http.StatusOK, "KYC updated", rider)
}

// GetOnboardingStatus returns onboarding completion status.
func (h *RiderHandler) GetOnboardingStatus(c *gin.Context) {
	userID := middleware.GetUserID(c)
	status, err := h.riderSvc.GetOnboardingStatus(c.Request.Context(), userID)
	if err != nil {
		dto.NotFound(c, "Rider not found")
		return
	}
	dto.Success(c, http.StatusOK, "onboarding status", status)
}

// GetDashboard returns the rider home screen data.
func (h *RiderHandler) GetDashboard(c *gin.Context) {
	userID := middleware.GetUserID(c)
	data, err := h.riderSvc.GetDashboard(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to load dashboard")
		return
	}
	dto.Success(c, http.StatusOK, "dashboard", data)
}

// GoOnline sets rider as available.
func (h *RiderHandler) GoOnline(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.riderSvc.GoOnline(c.Request.Context(), userID); err != nil {
		dto.InternalError(c, "Failed to go online")
		return
	}
	dto.Success(c, http.StatusOK, "you are now online", nil)
}

// GoOffline sets rider as unavailable.
func (h *RiderHandler) GoOffline(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.riderSvc.GoOffline(c.Request.Context(), userID); err != nil {
		dto.InternalError(c, "Failed to go offline")
		return
	}
	dto.Success(c, http.StatusOK, "you are now offline", nil)
}

// GetAvailability returns current availability.
func (h *RiderHandler) GetAvailability(c *gin.Context) {
	userID := middleware.GetUserID(c)
	available, onTrip, err := h.riderSvc.GetAvailability(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to get availability")
		return
	}
	dto.Success(c, http.StatusOK, "availability", gin.H{
		"is_available": available,
		"on_trip":      onTrip,
	})
}

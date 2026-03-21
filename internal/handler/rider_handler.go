package handler

import (
	"encoding/json"
	"net/http"
	"time"

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

	phone := ""
	if rider.Phone != nil {
		phone = *rider.Phone
	}

	firstName := ""
	if rider.FirstName != nil {
		firstName = *rider.FirstName
	}

	lastName := ""
	if rider.LastName != nil {
		lastName = *rider.LastName
	}

	vehicleType := ""
	if rider.VehicleType != nil {
		vehicleType = *rider.VehicleType
	}

	registrationNo := ""
	if rider.VehicleRegistrationNumber != nil {
		registrationNo = *rider.VehicleRegistrationNumber
	}

	status := "offline"
	if rider.IsAvailable {
		status = "online"
	}

	avgRating := 0.0
	if rider.RatingAvg != nil {
		avgRating = *rider.RatingAvg
	}

	resp := dto.RiderProfileResponse{
		User: dto.UserProfile{
			FirstName: firstName,
			LastName:  lastName,
			Phone:     phone,
			City:      "Pune", // Mocked statically for now
		},
		Rider: dto.RiderStats{
			AvgRating:          avgRating,
			AvailabilityStatus: status,
			ActiveHoursToday:   4.5, // Mocked dynamically
		},
		Vehicle: dto.VehicleProfile{
			VehicleType:    vehicleType,
			RegistrationNo: registrationNo,
		},
	}

	dto.Success(c, http.StatusOK, "profile fetched", resp)
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

	vehicleDetailsMap := map[string]interface{}{
		"make":            req.Make,
		"model":           req.Model,
		"year":            req.Year,
		"rc_document_url": req.RCDocumentURL,
	}
	vehicleDetailsBytes, _ := json.Marshal(vehicleDetailsMap)
	vehicleDetailsStr := string(vehicleDetailsBytes)

	insuranceDetailsMap := map[string]interface{}{
		"insurance_document_url": req.InsuranceDocumentURL,
	}
	insuranceBytes, _ := json.Marshal(insuranceDetailsMap)
	insuranceStr := string(insuranceBytes)

	rider, err := h.riderSvc.UpdateVehicle(c.Request.Context(), userID, &req.VehicleType, &req.RegistrationNumber, &vehicleDetailsStr, &insuranceStr, nil, nil, nil)
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

	bankDetailsBytes, _ := json.Marshal(req)
	bankDetailsStr := string(bankDetailsBytes)

	rider, err := h.riderSvc.UpdateBankDetails(c.Request.Context(), userID, &bankDetailsStr, nil)
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

	kycDataMap := map[string]interface{}{
		"national_id_type":   req.NationalIDType,
		"national_id_number": req.NationalIDNumber,
	}
	kycDataBytes, _ := json.Marshal(kycDataMap)
	kycDataStr := string(kycDataBytes)

	verificationDocsMap := map[string]interface{}{
		"driving_license_front_url": req.DrivingLicenseFrontURL,
		"driving_license_back_url":  req.DrivingLicenseBackURL,
		"national_id_front_url":     req.NationalIDFrontURL,
		"national_id_back_url":      req.NationalIDBackURL,
	}
	docsBytes, _ := json.Marshal(verificationDocsMap)
	docsStr := string(docsBytes)

	rider, err := h.riderSvc.UpdateKYC(c.Request.Context(), userID, &req.DrivingLicenseNumber, &kycDataStr, &docsStr)
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
	dto.Success(c, http.StatusOK, "Shift updated", nil)
}

// GoOffline sets rider as unavailable.
func (h *RiderHandler) GoOffline(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if err := h.riderSvc.GoOffline(c.Request.Context(), userID); err != nil {
		dto.InternalError(c, "Failed to go offline")
		return
	}
	dto.Success(c, http.StatusOK, "Shift updated", nil)
}

// GetAvailability returns current availability.
func (h *RiderHandler) GetAvailability(c *gin.Context) {
	userID := middleware.GetUserID(c)
	available, _, err := h.riderSvc.GetAvailability(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to get availability")
		return
	}

	var activeShift *dto.ActiveShift
	if available {
		// Mock shift start time to today at 8:00 AM
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
		activeShift = &dto.ActiveShift{StartedAt: startOfDay}
	}

	resp := dto.RiderAvailabilityResponse{
		IsAvailable: available,
		ActiveShift: activeShift,
	}

	dto.Success(c, http.StatusOK, "availability", resp)
}

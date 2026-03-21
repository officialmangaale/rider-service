package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// LocationHandler handles GPS location endpoints.
type LocationHandler struct {
	locationSvc *service.LocationService
}

// NewLocationHandler creates a new LocationHandler.
func NewLocationHandler(locationSvc *service.LocationService) *LocationHandler {
	return &LocationHandler{locationSvc: locationSvc}
}

// UpdateLocation updates the rider's GPS position.
func (h *LocationHandler) UpdateLocation(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req dto.UpdateLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "latitude and longitude are required")
		return
	}

	if err := h.locationSvc.UpdateLocation(c.Request.Context(), userID, req.Latitude, req.Longitude, req.Heading, req.Speed); err != nil {
		dto.InternalError(c, "Failed to update location")
		return
	}
	dto.Success(c, http.StatusOK, "location updated", nil)
}

// GetCurrentLocation returns the current position.
func (h *LocationHandler) GetCurrentLocation(c *gin.Context) {
	userID := middleware.GetUserID(c)
	loc, err := h.locationSvc.GetCurrentLocation(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to get location")
		return
	}
	dto.Success(c, http.StatusOK, "current location", loc)
}

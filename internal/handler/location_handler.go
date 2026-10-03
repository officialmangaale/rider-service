package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/debug"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// describeLocationSource makes a client-supplied label safe to log: only
// short lowercase identifiers pass through, so a crafted value cannot forge
// log lines. An absent value is "legacy" — an app build that predates it.
func describeLocationSource(value string) string {
	if value == "" {
		return "legacy"
	}
	if len(value) > 32 {
		return "other"
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && r != '_' {
			return "other"
		}
	}
	return value
}

func describeSequence(seq *int64) string {
	if seq == nil {
		return "-"
	}
	return strconv.FormatInt(*seq, 10)
}

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

	resp, err := h.locationSvc.UpdateRiderLocation(c.Request.Context(), userID, service.RiderLocationUpdate{
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
		Heading:        req.Heading,
		Speed:          req.Speed,
		AccuracyMeters: req.AccuracyMeters,
		RecordedAt:     req.RecordedAt,
		Source:         req.Source,
		AppState:       req.AppState,
		Sequence:       req.Sequence,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidRiderLocation) {
			log.Printf("[LOCATION] Rejected invalid rider location rider_id=%s source=%s", userID, describeLocationSource(req.Source))
			dto.ValidationError(c, "invalid rider location coordinates")
			return
		}
		log.Printf("[LOCATION] Update failed rider_id=%s source=%s err=%v", userID, describeLocationSource(req.Source), err)
		dto.InternalError(c, "Failed to update location")
		return
	}
	// Per-update logging is debug-only: every online rider sends one every
	// 15-45 seconds. Coordinates are never logged.
	debug.Logf("[LOCATION] Update received rider_id=%s source=%s app_state=%s seq=%s",
		userID, describeLocationSource(req.Source), describeLocationSource(req.AppState), describeSequence(req.Sequence))
	dto.Success(c, http.StatusOK, "Location updated successfully", resp)
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

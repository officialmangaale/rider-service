package handler

import (
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

// InternalRestaurantAssignment accepts a hint, then verifies shared DB ownership
// through ProcessRiderAssignedEvent before producing any delivery or push.
func (h *DeliveryHandler) InternalRestaurantAssignment(c *gin.Context) {
	var event models.RiderAssignedToOrderEvent
	if c.ShouldBindJSON(&event) != nil || event.OrderID <= 0 || event.RestaurantID <= 0 || strings.TrimSpace(event.RiderUserID) == "" {
		dto.ValidationError(c, "Invalid restaurant assignment")
		return
	}
	if err := h.deliverySvc.ProcessRiderAssignedEvent(c.Request.Context(), &event); err != nil {
		dto.InternalError(c, "Could not deliver restaurant assignment")
		return
	}
	dto.Success(c, http.StatusOK, "Restaurant assignment reconciled", gin.H{"order_id": event.OrderID})
}

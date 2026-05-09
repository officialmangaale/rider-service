package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// DeliveryHandler handles the new delivery assignment and tracking APIs.
type DeliveryHandler struct {
	deliverySvc *service.DeliveryService
}

func NewDeliveryHandler(deliverySvc *service.DeliveryService) *DeliveryHandler {
	return &DeliveryHandler{deliverySvc: deliverySvc}
}

// UpdateLocation handles POST /riders/location
func (h *DeliveryHandler) UpdateLocation(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	var req struct {
		Latitude  float64 `json:"latitude" binding:"required"`
		Longitude float64 `json:"longitude" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "latitude and longitude are required")
		return
	}
	if err := h.deliverySvc.UpdateRiderLocation(c.Request.Context(), riderID, req.Latitude, req.Longitude); err != nil {
		dto.InternalError(c, "Failed to update location")
		return
	}
	dto.Success(c, http.StatusOK, "Location updated", nil)
}

// UpdateAvailability handles POST /riders/availability
func (h *DeliveryHandler) UpdateAvailability(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	var req struct {
		IsOnline    bool `json:"is_online"`
		IsAvailable bool `json:"is_available"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}
	if err := h.deliverySvc.UpdateRiderAvailability(c.Request.Context(), riderID, req.IsOnline, req.IsAvailable); err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "Availability updated", nil)
}

// GetOrderRequests handles GET /riders/order-requests
func (h *DeliveryHandler) GetOrderRequests(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	reqs, err := h.deliverySvc.GetPendingRequests(c.Request.Context(), riderID)
	if err != nil {
		dto.InternalError(c, "Failed to fetch order requests")
		return
	}
	if reqs == nil {
		reqs = []*models.DeliveryOrderRequest{}
	}
	dto.Success(c, http.StatusOK, "order requests", reqs)
}

// AcceptRequest handles POST /riders/order-requests/{requestId}/accept
func (h *DeliveryHandler) AcceptRequest(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	requestID, err := strconv.Atoi(c.Param("requestId"))
	if err != nil {
		dto.ValidationError(c, "Invalid request ID")
		return
	}
	if err := h.deliverySvc.AcceptRequest(c.Request.Context(), requestID, riderID); err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "Request accepted, rider assigned", nil)
}

// RejectRequest handles POST /riders/order-requests/{requestId}/reject
func (h *DeliveryHandler) RejectRequest(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	requestID, err := strconv.Atoi(c.Param("requestId"))
	if err != nil {
		dto.ValidationError(c, "Invalid request ID")
		return
	}
	if err := h.deliverySvc.RejectRequest(c.Request.Context(), requestID, riderID); err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "Request rejected", nil)
}

// UpdateDeliveryStatus handles POST /riders/orders/{orderId}/status
func (h *DeliveryHandler) UpdateDeliveryStatus(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("orderId"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}
	var req struct {
		DeliveryStatus   string `json:"delivery_status" binding:"required"`
		PaymentCollected bool   `json:"payment_collected"`
		Notes            string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "delivery_status is required")
		return
	}

	// Validate allowed values
	allowed := map[string]bool{
		"rider_arrived_restaurant": true,
		"picked_up":               true,
		"on_the_way":              true,
		"delivered":               true,
	}
	if !allowed[req.DeliveryStatus] {
		dto.ValidationError(c, "Invalid delivery_status. Allowed: rider_arrived_restaurant, picked_up, on_the_way, delivered")
		return
	}

	if err := h.deliverySvc.UpdateDeliveryStatus(c.Request.Context(), orderID, riderID, req.DeliveryStatus, req.PaymentCollected, req.Notes); err != nil {
		if err.Error() == "cash collection confirmation required" {
			dto.ErrorResponse(c, http.StatusBadRequest, "cash collection confirmation required", err.Error())
			return
		}
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "Delivery status updated", nil)
}

// GetDeliveryTracking handles GET /delivery/orders/{orderId}/tracking
func (h *DeliveryHandler) GetDeliveryTracking(c *gin.Context) {
	orderID, err := strconv.Atoi(c.Param("orderId"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}
	tracking, err := h.deliverySvc.GetDeliveryTracking(c.Request.Context(), orderID)
	if err != nil {
		dto.NotFound(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "delivery tracking", tracking)
}

// GetRiderOrders returns restaurant-owned (or platform) orders for the active rider.
func (h *DeliveryHandler) GetRiderOrders(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	statusParam := c.Query("status") // comma separated
	var statuses []string
	if statusParam != "" {
		// Just passing a single status for now, or could split by comma
		statuses = append(statuses, statusParam)
	} else {
		statuses = []string{"rider_assigned", "rider_arrived_restaurant", "picked_up", "on_the_way", "delivered"}
	}

	orders, err := h.deliverySvc.GetRiderOrders(c.Request.Context(), riderID, statuses)
	if err != nil {
		dto.InternalError(c, "Failed to fetch orders")
		return
	}

	var res []dto.RiderDeliveryOrderResponse
	for _, o := range orders {
		res = append(res, mapDeliveryOrderToDTO(o))
	}
	if res == nil {
		res = []dto.RiderDeliveryOrderResponse{}
	}
	dto.Success(c, http.StatusOK, "Rider orders", res)
}

// GetRiderOrderDetail returns a specific delivery order for the rider.
func (h *DeliveryHandler) GetRiderOrderDetail(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("orderId"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	do, err := h.deliverySvc.GetRiderOrderDetail(c.Request.Context(), orderID, riderID)
	if err != nil {
		dto.NotFound(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "Order detail", mapDeliveryOrderToDTO(do))
}

func mapDeliveryOrderToDTO(o *models.DeliveryOrder) dto.RiderDeliveryOrderResponse {
	var mapsURL string
	if o.DropLatitude != 0 && o.DropLongitude != 0 {
		mapsURL = "https://www.google.com/maps/dir/?api=1&destination=" + strconv.FormatFloat(o.DropLatitude, 'f', -1, 64) + "," + strconv.FormatFloat(o.DropLongitude, 'f', -1, 64)
	}
	
	assignmentType := o.AssignmentType
	if assignmentType == "" {
		assignmentType = "platform" // Default
	}

	return dto.RiderDeliveryOrderResponse{
		OrderID:         o.OrderID,
		DeliveryOrderID: o.DeliveryOrderID,
		RestaurantID:    o.RestaurantID,
		RestaurantName:  o.RestaurantName,
		RestaurantPhone: o.RestaurantPhone,
		DeliveryStatus:  o.DeliveryStatus,
		PaymentMethod:   o.PaymentMode,
		AmountToCollect: o.Amount, // Could add logic if prepaid
		Customer: &dto.CustomerInfo{
			Name:  "Customer", // To fetch from user profile if needed
			Phone: "",         // To fetch
		},
		DeliveryAddress: &dto.DeliveryAddressInfo{
			Address:   o.DropAddress,
			Latitude:  o.DropLatitude,
			Longitude: o.DropLongitude,
		},
		PickupAddress: &dto.PickupAddressInfo{
			Address:   o.PickupAddress,
			Latitude:  o.PickupLatitude,
			Longitude: o.PickupLongitude,
		},
		ItemsSummary:    "Items", // To fetch from order items
		MapsURL:         mapsURL,
		AssignmentType:  assignmentType,
		AssignedAt:      o.AssignedAt,
	}
}

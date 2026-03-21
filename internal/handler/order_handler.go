package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// OrderHandler handles order assignment and delivery lifecycle endpoints.
type OrderHandler struct {
	orderSvc *service.OrderService
}

// NewOrderHandler creates a new OrderHandler.
func NewOrderHandler(orderSvc *service.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

// GetAvailableOrders lists ready-for-pickup orders.
func (h *OrderHandler) GetAvailableOrders(c *gin.Context) {
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	orders, total, err := h.orderSvc.GetAvailableOrders(c.Request.Context(), pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch available orders")
		return
	}
	dto.Paginated(c, orders, pq.Page, pq.Limit, total)
}

// GetActiveOrder returns the rider's current active delivery.
func (h *OrderHandler) GetActiveOrder(c *gin.Context) {
	userID := middleware.GetUserID(c)
	order, err := h.orderSvc.GetActiveOrder(c.Request.Context(), userID)
	if err != nil {
		dto.Success(c, http.StatusOK, "no active order", nil)
		return
	}
	dto.Success(c, http.StatusOK, "active order", order)
}

// GetIncomingAssignment returns the current pending assignment offer.
func (h *OrderHandler) GetIncomingAssignment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	assignment, order, err := h.orderSvc.GetIncomingAssignment(c.Request.Context(), userID)
	if err != nil {
		dto.Success(c, http.StatusOK, "no incoming assignment", nil)
		return
	}
	dto.Success(c, http.StatusOK, "incoming assignment", gin.H{
		"assignment": assignment,
		"order":      order,
	})
}

// AcceptAssignment accepts a delivery assignment.
func (h *OrderHandler) AcceptAssignment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	assignmentID := c.Param("id")
	if assignmentID == "" {
		dto.ValidationError(c, "Assignment ID required")
		return
	}

	order, err := h.orderSvc.AcceptAssignment(c.Request.Context(), assignmentID, userID)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "assignment accepted", order)
}

// RejectAssignment rejects a delivery assignment.
func (h *OrderHandler) RejectAssignment(c *gin.Context) {
	userID := middleware.GetUserID(c)
	assignmentID := c.Param("id")
	var req dto.RejectAssignmentRequest
	_ = c.ShouldBindJSON(&req)

	if err := h.orderSvc.RejectAssignment(c.Request.Context(), assignmentID, userID, req.Reason); err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "assignment rejected", nil)
}

// GetOrderDetail returns a specific order.
func (h *OrderHandler) GetOrderDetail(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	order, err := h.orderSvc.GetOrderDetail(c.Request.Context(), orderID, userID)
	if err != nil {
		dto.NotFound(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "order detail", order)
}

// GetOrderHistory returns paginated past orders.
func (h *OrderHandler) GetOrderHistory(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	orders, total, err := h.orderSvc.GetHistory(c.Request.Context(), userID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch order history")
		return
	}
	dto.Paginated(c, orders, pq.Page, pq.Limit, total)
}

// PickedUp marks order as out_for_delivery.
func (h *OrderHandler) PickedUp(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	order, err := h.orderSvc.PickedUp(c.Request.Context(), orderID, userID)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "order picked up", order)
}

// ArrivedAtRestaurant records that rider reached the restaurant.
func (h *OrderHandler) ArrivedAtRestaurant(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	order, err := h.orderSvc.ArrivedAtRestaurant(c.Request.Context(), orderID, userID)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "arrived at restaurant", order)
}

// ArrivedAtCustomer records that rider reached the customer location.
func (h *OrderHandler) ArrivedAtCustomer(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	order, err := h.orderSvc.ArrivedAtCustomer(c.Request.Context(), orderID, userID)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "arrived at customer", order)
}

// Delivered marks order as delivered.
func (h *OrderHandler) Delivered(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	order, err := h.orderSvc.Delivered(c.Request.Context(), orderID, userID)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "order delivered", order)
}

// CancelDelivery cancels the delivery.
func (h *OrderHandler) CancelDelivery(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	var req dto.CancelDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Reason is required")
		return
	}

	order, err := h.orderSvc.CancelDelivery(c.Request.Context(), orderID, userID, req.Reason)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "delivery cancelled", order)
}

// FailDelivery marks delivery as failed.
func (h *OrderHandler) FailDelivery(c *gin.Context) {
	userID := middleware.GetUserID(c)
	orderID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		dto.ValidationError(c, "Invalid order ID")
		return
	}

	var req dto.FailDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Reason is required")
		return
	}

	order, err := h.orderSvc.FailDelivery(c.Request.Context(), orderID, userID, req.Reason)
	if err != nil {
		dto.Conflict(c, err.Error())
		return
	}
	dto.Success(c, http.StatusOK, "delivery failed", order)
}

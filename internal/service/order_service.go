package service

import (
	"context"
	"fmt"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/constants"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// OrderService handles order assignment and delivery lifecycle.
type OrderService struct {
	orderRepo      *repository.OrderRepository
	assignmentRepo *repository.AssignmentRepository
	riderRepo      *repository.RiderRepository
	earningsRepo   *repository.EarningsRepository
	historyRepo    *repository.StatusHistoryRepository
}

// NewOrderService creates a new OrderService.
func NewOrderService(
	orderRepo *repository.OrderRepository,
	assignmentRepo *repository.AssignmentRepository,
	riderRepo *repository.RiderRepository,
	earningsRepo *repository.EarningsRepository,
	historyRepo *repository.StatusHistoryRepository,
) *OrderService {
	return &OrderService{
		orderRepo:      orderRepo,
		assignmentRepo: assignmentRepo,
		riderRepo:      riderRepo,
		earningsRepo:   earningsRepo,
		historyRepo:    historyRepo,
	}
}

// GetAvailableOrders returns delivery orders ready for pickup.
func (s *OrderService) GetAvailableOrders(ctx context.Context, limit, offset int) ([]*models.Order, int64, error) {
	return s.orderRepo.GetAvailableOrders(ctx, limit, offset)
}

// GetActiveOrder returns the rider's current active delivery.
func (s *OrderService) GetActiveOrder(ctx context.Context, riderID string) (*models.Order, error) {
	return s.orderRepo.GetActiveOrderForRider(ctx, riderID)
}

// GetIncomingAssignment returns the rider's pending assignment offer.
func (s *OrderService) GetIncomingAssignment(ctx context.Context, riderID string) (*models.DeliveryAssignment, *models.Order, error) {
	assignment, err := s.assignmentRepo.GetPendingForRider(ctx, riderID)
	if err != nil {
		return nil, nil, err
	}
	order, err := s.orderRepo.GetOrderByID(ctx, assignment.OrderID)
	if err != nil {
		return assignment, nil, err
	}
	return assignment, order, nil
}

// AcceptAssignment accepts a delivery assignment with safe claim.
func (s *OrderService) AcceptAssignment(ctx context.Context, assignmentID, riderID string) (*models.Order, error) {
	// Get the assignment
	assignment, err := s.assignmentRepo.GetByID(ctx, assignmentID)
	if err != nil {
		return nil, fmt.Errorf("assignment not found")
	}
	if assignment.RiderID != riderID {
		return nil, fmt.Errorf("assignment does not belong to this rider")
	}
	if assignment.Status != string(constants.AssignmentPending) {
		return nil, fmt.Errorf("assignment already responded to")
	}

	// Begin transaction
	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Mark assignment as accepted
	if err := s.assignmentRepo.Accept(ctx, tx, assignmentID); err != nil {
		return nil, fmt.Errorf("failed to accept assignment")
	}

	// Claim the order — set delivery_partner_id atomically
	if err := s.orderRepo.AssignRiderToOrder(ctx, tx, assignment.OrderID, riderID); err != nil {
		return nil, err // "order already assigned to another rider"
	}

	// Set rider as on_trip
	if err := s.riderRepo.SetOnTripTx(ctx, tx, riderID, true); err != nil {
		return nil, err
	}

	// Record status history
	if err := s.historyRepo.Record(ctx, tx, assignment.OrderID, string(constants.OrderReady), "assigned_to_rider", riderID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.orderRepo.GetOrderByID(ctx, assignment.OrderID)
}

// RejectAssignment rejects an assignment with a reason.
func (s *OrderService) RejectAssignment(ctx context.Context, assignmentID, riderID, reason string) error {
	assignment, err := s.assignmentRepo.GetByID(ctx, assignmentID)
	if err != nil {
		return fmt.Errorf("assignment not found")
	}
	if assignment.RiderID != riderID {
		return fmt.Errorf("assignment does not belong to this rider")
	}
	return s.assignmentRepo.Reject(ctx, assignmentID, reason)
}

// PickedUp marks an order as out_for_delivery (rider picked up from restaurant).
func (s *OrderService) PickedUp(ctx context.Context, orderID int, riderID string) (*models.Order, error) {
	return s.transitionOrder(ctx, orderID, riderID, constants.OrderReady, constants.OrderOutForDelivery)
}

// Delivered marks an order as delivered and creates earnings.
func (s *OrderService) Delivered(ctx context.Context, orderID int, riderID string) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if order.DeliveryPartnerID == nil || *order.DeliveryPartnerID != riderID {
		return nil, fmt.Errorf("order not assigned to this rider")
	}
	if !constants.IsRiderAllowedTransition(constants.OrderStatus(order.OrderStatus), constants.OrderDelivered) {
		return nil, fmt.Errorf("cannot deliver: order is in '%s' state", order.OrderStatus)
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Update status with optimistic lock
	if err := s.orderRepo.UpdateOrderStatus(ctx, tx, orderID, order.OrderStatus, string(constants.OrderDelivered)); err != nil {
		return nil, err
	}

	// Set actual delivery time
	if err := s.orderRepo.MarkDelivered(ctx, tx, orderID); err != nil {
		return nil, err
	}

	// Record audit history
	if err := s.historyRepo.Record(ctx, tx, orderID, order.OrderStatus, string(constants.OrderDelivered), riderID); err != nil {
		return nil, err
	}

	// Create delivery fee earning
	if order.DeliveryFee > 0 {
		if err := s.earningsRepo.Create(ctx, tx, riderID, &orderID, string(constants.EarningDeliveryFee), order.DeliveryFee, "Delivery fee"); err != nil {
			return nil, err
		}
	}

	// Create tip earning
	if order.TipAmount > 0 {
		if err := s.earningsRepo.Create(ctx, tx, riderID, &orderID, string(constants.EarningTip), order.TipAmount, "Customer tip"); err != nil {
			return nil, err
		}
	}

	// Update rider stats: increment delivery count, add earnings, clear on_trip
	totalEarning := order.DeliveryFee + order.TipAmount
	if err := s.riderRepo.IncrementDeliveryCount(ctx, tx, riderID, totalEarning); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.orderRepo.GetOrderByID(ctx, orderID)
}

// CancelDelivery cancels the delivery with a reason.
func (s *OrderService) CancelDelivery(ctx context.Context, orderID int, riderID, reason string) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if order.DeliveryPartnerID == nil || *order.DeliveryPartnerID != riderID {
		return nil, fmt.Errorf("order not assigned to this rider")
	}

	// Rider can cancel from ready or out_for_delivery
	currentStatus := constants.OrderStatus(order.OrderStatus)
	if currentStatus != constants.OrderReady && currentStatus != constants.OrderOutForDelivery {
		return nil, fmt.Errorf("cannot cancel: order is in '%s' state", order.OrderStatus)
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Update order status
	if err := s.orderRepo.UpdateOrderStatus(ctx, tx, orderID, order.OrderStatus, string(constants.OrderCancelled)); err != nil {
		return nil, err
	}

	// Unassign rider
	if err := s.orderRepo.UnassignRider(ctx, tx, orderID); err != nil {
		return nil, err
	}

	// Clear on_trip
	if err := s.riderRepo.SetOnTripTx(ctx, tx, riderID, false); err != nil {
		return nil, err
	}

	// Record audit history with reason
	if err := s.historyRepo.Record(ctx, tx, orderID, order.OrderStatus, string(constants.OrderCancelled)+" ("+reason+")", riderID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.orderRepo.GetOrderByID(ctx, orderID)
}

// ArrivedAtRestaurant records that the rider reached the restaurant.
// This is a rider-side tracking event — it does NOT change the shared order_status.
// Idempotent: recording the event multiple times is safe.
func (s *OrderService) ArrivedAtRestaurant(ctx context.Context, orderID int, riderID string) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if order.DeliveryPartnerID == nil || *order.DeliveryPartnerID != riderID {
		return nil, fmt.Errorf("order not assigned to this rider")
	}
	// Valid when order is 'ready' (assigned but not yet picked up)
	if constants.OrderStatus(order.OrderStatus) != constants.OrderReady {
		return nil, fmt.Errorf("cannot mark arrived at restaurant: order is in '%s' state", order.OrderStatus)
	}
	// Record event in delivery_status_history (no order_status change)
	_ = s.historyRepo.Record(ctx, nil, orderID, order.OrderStatus, string(constants.RiderEventArrivedAtRestaurant), riderID)
	return order, nil
}

// ArrivedAtCustomer records that the rider reached the customer location.
// This is a rider-side tracking event — it does NOT change the shared order_status.
// Idempotent: recording the event multiple times is safe.
func (s *OrderService) ArrivedAtCustomer(ctx context.Context, orderID int, riderID string) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if order.DeliveryPartnerID == nil || *order.DeliveryPartnerID != riderID {
		return nil, fmt.Errorf("order not assigned to this rider")
	}
	// Valid when order is 'out_for_delivery'
	if constants.OrderStatus(order.OrderStatus) != constants.OrderOutForDelivery {
		return nil, fmt.Errorf("cannot mark arrived at customer: order is in '%s' state", order.OrderStatus)
	}
	// Record event in delivery_status_history (no order_status change)
	_ = s.historyRepo.Record(ctx, nil, orderID, order.OrderStatus, string(constants.RiderEventArrivedAtCustomer), riderID)
	return order, nil
}

// FailDelivery marks delivery as failed (e.g., customer unreachable).
func (s *OrderService) FailDelivery(ctx context.Context, orderID int, riderID, reason string) (*models.Order, error) {
	// Same flow as cancel for now — can be extended with different status if needed
	return s.CancelDelivery(ctx, orderID, riderID, "FAILED: "+reason)
}

// GetOrderDetail returns a specific order for the rider.
func (s *OrderService) GetOrderDetail(ctx context.Context, orderID int, riderID string) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// Verify this rider has access
	if order.DeliveryPartnerID == nil || *order.DeliveryPartnerID != riderID {
		return nil, fmt.Errorf("order not accessible")
	}
	return order, nil
}

// GetHistory returns paginated order history for the rider.
func (s *OrderService) GetHistory(ctx context.Context, riderID string, limit, offset int) ([]*models.Order, int64, error) {
	return s.orderRepo.GetOrderHistoryForRider(ctx, riderID, limit, offset)
}

// transitionOrder is a helper for simple status transitions.
func (s *OrderService) transitionOrder(ctx context.Context, orderID int, riderID string, expectedFrom, newTo constants.OrderStatus) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	if order.DeliveryPartnerID == nil || *order.DeliveryPartnerID != riderID {
		return nil, fmt.Errorf("order not assigned to this rider")
	}
	if !constants.IsRiderAllowedTransition(constants.OrderStatus(order.OrderStatus), newTo) {
		return nil, fmt.Errorf("cannot transition: order is in '%s' state, expected '%s'", order.OrderStatus, expectedFrom)
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.orderRepo.UpdateOrderStatus(ctx, tx, orderID, order.OrderStatus, string(newTo)); err != nil {
		return nil, err
	}

	if err := s.historyRepo.Record(ctx, tx, orderID, order.OrderStatus, string(newTo), riderID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.orderRepo.GetOrderByID(ctx, orderID)
}

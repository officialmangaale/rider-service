package service

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// Older rider builds use /delivery/:id/* rather than /riders/orders/:id/status.
// Route deliveries in the existing projection through the same lifecycle.
func (s *OrderService) transitionDeliveryProjection(ctx context.Context, orderID int, riderID, next string, collected bool, notes string) (bool, error) {
	if s.deliverySvc == nil {
		return false, nil
	}
	order, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID, "food")
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if next == "picked_up" && order.DeliveryStatus == models.DeliveryStatusRiderAssigned {
		if err := s.deliverySvc.UpdateDeliveryStatus(ctx, orderID, riderID, "rider_arrived_restaurant", false, "", "food"); err != nil {
			return true, err
		}
	}
	if next == "delivered" && order.DeliveryStatus == models.DeliveryStatusPickedUp {
		if err := s.deliverySvc.UpdateDeliveryStatus(ctx, orderID, riderID, "on_the_way", false, "", "food"); err != nil {
			return true, err
		}
	}
	return true, s.deliverySvc.UpdateDeliveryStatus(ctx, orderID, riderID, next, collected, notes, "food")
}

func (s *DeliveryService) WithdrawDelivery(ctx context.Context, orderID int, riderID, reason string) error {
	if err := s.deliveryRepo.WithdrawFoodDelivery(ctx, orderID, riderID, reason); err != nil {
		return err
	}
	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		s.dispatchCache.ReleaseOrderLock(ctx, orderID, riderID)
	}
	_ = s.riderRepo.SetOnTrip(ctx, riderID, false)
	return nil
}

// A rounded straight-line distance is sufficient to evaluate an offer without
// disclosing the customer's address/coordinates or calling a paid routing API.
func approximateDeliveryDistance(order *models.DeliveryOrder) float64 {
	rad := math.Pi / 180
	a, b := order.PickupLatitude*rad, order.DropLatitude*rad
	c := math.Sin(a)*math.Sin(b) + math.Cos(a)*math.Cos(b)*math.Cos((order.DropLongitude-order.PickupLongitude)*rad)
	return math.Round(6371 * math.Acos(math.Max(-1, math.Min(1, c))))
}

// ReconcileFoodDispatches repairs missed SQS publishes using the existing
// delivery_orders projection. Bounded by the same search window and batch size.
func (s *DeliveryService) ReconcileFoodDispatches(ctx context.Context) error {
	// Repair a canonical pickup/completion whose HTTP response or projection
	// write was lost, even with the dispatch flag disabled.
	repairs, err := s.deliveryRepo.FoodProgressRepairs(ctx)
	if err != nil {
		return err
	}
	for _, repair := range repairs {
		order, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, repair.DeliveryOrderID)
		if err != nil {
			return err
		}
		if _, err := s.deliveryRepo.AdvanceDelivery(ctx, order, repair.RiderID, repair.Target); err != nil {
			return err
		}
		if repair.Target == "delivered" {
			_ = s.riderRepo.SetOnTrip(ctx, repair.RiderID, false)
			if available, err := s.deliveryRepo.GetRiderAvailability(ctx, repair.RiderID); err == nil {
				_ = s.riderRepo.SetAvailability(ctx, repair.RiderID, available.IsOnline)
			}
		}
	}
	if err := s.deliveryRepo.ExpireFoodSearches(ctx); err != nil {
		return err
	}
	ids, err := s.deliveryRepo.MissingFoodDispatches(ctx, 50)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.ProcessOrderPlacedEvent(ctx, &models.OrderPlacedEvent{OrderID: id, SourceOrderType: "food"}); err != nil {
			return err
		}
	}
	return nil
}

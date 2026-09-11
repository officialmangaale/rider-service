package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// The delivery lifecycle after a rider accepts.
//
// Two records describe one delivery:
//   - delivery_orders (this service): rider_assigned → rider_arrived_restaurant
//     → picked_up → on_the_way → delivered.
//   - orders (restaurant-service, what the customer and owner apps read):
//     the canonical order_status plus the assigned rider.
//
// A rider's picked_up / on_the_way / delivered must first succeed on the
// canonical order (restaurant-service allows a rider only ready →
// out_for_delivery → delivered), and restaurant-service accepts it only from
// the rider recorded on that order. So before those steps this file makes sure
// the rider is recorded there, and that the kitchen has marked the order ready.

// Error codes sent to the rider app in `error_code`.
const (
	ErrCodeDeliveryNotFound  = "DELIVERY_NOT_FOUND"
	ErrCodeNotAssignedRider  = "NOT_ASSIGNED_RIDER"
	ErrCodeInvalidTransition = "INVALID_TRANSITION"
	ErrCodeCashNotConfirmed  = "CASH_COLLECTION_REQUIRED"
	ErrCodeOrderNotReady     = "ORDER_NOT_READY"
	ErrCodeRestaurantSync    = "RESTAURANT_SYNC_FAILED"
	ErrCodeStatusWriteFailed = "STATUS_UPDATE_FAILED"
	ErrCodeOrderClosed       = "ORDER_CLOSED"
)

// DeliveryStatusError is a refused rider status update. Message keeps the
// wording older app builds already handle.
type DeliveryStatusError struct {
	Code          string
	Message       string
	CurrentStatus string
}

func (e *DeliveryStatusError) Error() string { return e.Message }

// requiresRestaurantTransition: statuses that move the canonical order.
func requiresRestaurantTransition(status string) bool {
	switch status {
	case models.DeliveryStatusPickedUp, models.DeliveryStatusOnTheWay, models.DeliveryStatusDelivered:
		return true
	}
	return false
}

// nextDeliveryStatus is the single valid next step, "" when none.
func nextDeliveryStatus(order *models.DeliveryOrder) string {
	if order.RestaurantOwned {
		switch order.DeliveryStatus {
		case models.DeliveryStatusRiderAssigned, models.DeliveryStatusRiderArrivedRestaurant:
			return models.DeliveryStatusPickedUp
		}
	}
	if next := models.DeliveryStatusTransitions[order.DeliveryStatus]; len(next) > 0 {
		return next[0]
	}
	return ""
}

// normalizeRestaurantOrderStatus mirrors the spellings restaurant-service's
// NormalizeOrderStatus accepts (services/order_status_contract.go).
func normalizeRestaurantOrderStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "placed":
		return "pending"
	case "accepted":
		return "confirmed"
	case "ready_to_serve", "readytoserve":
		return "ready"
	case "picked_up", "on_the_way":
		return "out_for_delivery"
	}
	return s
}

// restaurantOrderClosed: the owner ended the order (completed, cancelled or
// rejected), so the rider can no longer progress this delivery. Matches
// repository.closedRestaurantStatuses.
func restaurantOrderClosed(orderStatus string) bool {
	switch normalizeRestaurantOrderStatus(orderStatus) {
	case "completed", "cancelled", "canceled", "rejected", "declined":
		return true
	}
	return false
}

// restaurantReadyForPickup: restaurant-service lets a rider move an order to
// out_for_delivery only from ready (or repeat it once there).
func restaurantReadyForPickup(orderStatus string) bool {
	switch normalizeRestaurantOrderStatus(orderStatus) {
	case "ready", "out_for_delivery":
		return true
	}
	return false
}

// rejectStatus logs a refused update and returns the error for the app.
func rejectStatus(orderID int, riderID, current, requested, code, reason, message string, extra dispatchtrace.Fields) error {
	fields := dispatchtrace.Fields{
		"order_id":         orderID,
		"rider_id":         riderID,
		"current_status":   current,
		"requested_status": requested,
		"reason_code":      reason,
	}
	for k, v := range extra {
		fields[k] = v
	}
	dispatchtrace.Emit(dispatchtrace.EventDeliveryStatusRejected, fields)
	return &DeliveryStatusError{Code: code, Message: message, CurrentStatus: current}
}

// prepareRestaurantTransition runs before a canonical status change. It
// refuses a pickup the kitchen has not released, and records the rider on the
// order if the acceptance callback never landed.
// snap is nil when the order row could not be read; restaurant-service still
// enforces both rules itself.
func (s *DeliveryService) prepareRestaurantTransition(ctx context.Context, order *models.DeliveryOrder, riderID, newStatus string, snap *repository.OrderRiderSnapshot) error {
	if snap == nil {
		return nil
	}
	if newStatus == models.DeliveryStatusPickedUp && !restaurantReadyForPickup(snap.OrderStatus) {
		return rejectStatus(order.OrderID, riderID, order.DeliveryStatus, newStatus,
			ErrCodeOrderNotReady, dispatchtrace.ReasonOrderNotReady,
			"order is not ready for pickup yet",
			dispatchtrace.Fields{"restaurant_status": snap.OrderStatus})
	}
	if order.RestaurantOwned || snap.HasRider(riderID) {
		return nil
	}
	assignedAt := time.Now()
	if order.AssignedAt != nil {
		assignedAt = *order.AssignedAt
	}
	if err := s.syncRiderAssignment(ctx, order.OrderID, riderID, assignedAt, "status_update"); err != nil {
		return rejectStatus(order.OrderID, riderID, order.DeliveryStatus, newStatus,
			ErrCodeRestaurantSync, dispatchtrace.ReasonAssignmentSyncFailed,
			"could not record the rider on the order, please try again",
			dispatchtrace.Fields{"http_status": callbackStatus(err)})
	}
	return nil
}

// ensureAssignmentRecorded is the best-effort version for steps that do not
// touch the canonical order (reached restaurant): it lets the customer see
// the rider even when the acceptance callback failed, and never blocks.
func (s *DeliveryService) ensureAssignmentRecorded(ctx context.Context, order *models.DeliveryOrder, riderID string, snap *repository.OrderRiderSnapshot) {
	if order.RestaurantOwned || s.restaurantCli == nil || snap == nil || snap.HasRider(riderID) {
		return
	}
	assignedAt := time.Now()
	if order.AssignedAt != nil {
		assignedAt = *order.AssignedAt
	}
	_ = s.syncRiderAssignment(ctx, order.OrderID, riderID, assignedAt, "status_update")
}

// syncRiderAssignment records the rider on restaurant-service's order.
// Idempotent there: the same rider again returns 200.
func (s *DeliveryService) syncRiderAssignment(ctx context.Context, orderID int, riderID string, assignedAt time.Time, trigger string) error {
	if s.restaurantCli == nil {
		err := errors.New("restaurant-service client not configured")
		emitAssignmentSyncFailed(orderID, riderID, trigger, err)
		return err
	}
	payload := s.assignmentPayload(ctx, riderID, assignedAt)
	if err := s.restaurantCli.NotifyRiderAssigned(orderID, payload); err != nil {
		emitAssignmentSyncFailed(orderID, riderID, trigger, err)
		return err
	}
	dispatchtrace.Emit(dispatchtrace.EventAssignmentSynced, dispatchtrace.Fields{
		"order_id": orderID,
		"rider_id": riderID,
		"trigger":  trigger,
		"has_name": payload.RiderName != "",
	})
	return nil
}

// orderSnapshot reads the restaurant order, or nil (logged) when it cannot.
func (s *DeliveryService) orderSnapshot(ctx context.Context, orderID int) *repository.OrderRiderSnapshot {
	snap, err := s.deliveryRepo.GetOrderRiderSnapshot(ctx, orderID)
	if err != nil {
		log.Printf("[DELIVERY] order snapshot unavailable order_id=%d err=%v", orderID, err)
		return nil
	}
	return &snap
}

// ReleaseClosedDeliveries frees riders whose active delivery belongs to an
// order the restaurant has already ended. Without it the rider keeps
// current_order_id and is excluded from every future offer: in production,
// order 13356 was completed by the owner mid-delivery and its rider received
// no offers afterwards. Called periodically by worker.ClosedDeliveryWorker.
func (s *DeliveryService) ReleaseClosedDeliveries(ctx context.Context) (int, error) {
	closed, err := s.deliveryRepo.FindDeliveriesClosedByRestaurant(ctx, 50)
	if err != nil {
		return 0, err
	}
	released := 0
	for _, c := range closed {
		if s.releaseClosedDelivery(ctx, c) {
			released++
		}
	}
	return released, nil
}

func (s *DeliveryService) releaseClosedDelivery(ctx context.Context, c repository.ClosedDelivery) bool {
	ok, err := s.deliveryRepo.ReleaseClosedDelivery(ctx, c)
	if err != nil {
		log.Printf("[DELIVERY] release of closed delivery failed order_id=%d err=%v", c.OrderID, err)
		return false
	}
	if !ok {
		return false
	}
	if c.RiderID != "" {
		_ = s.riderRepo.SetOnTrip(ctx, c.RiderID, false)
		if s.dispatchCache != nil && s.dispatchCache.Enabled() {
			if err := s.dispatchCache.UpdateRiderAvailability(ctx, c.RiderID, true, true, nil); err != nil {
				log.Printf("[DELIVERY] Redis rider free update failed rider_id=%s order_id=%d err=%v", c.RiderID, c.OrderID, err)
			}
		}
	}
	dispatchtrace.Emit(dispatchtrace.EventDeliveryReleased, dispatchtrace.Fields{
		"order_id":          c.OrderID,
		"delivery_order_id": c.DeliveryOrderID,
		"rider_id":          c.RiderID,
		"restaurant_status": c.OrderStatus,
		"reason_code":       dispatchtrace.ReasonRestaurantClosedOrder,
	})
	return true
}

// assignmentRetryDelays: waits between acceptance-time sync attempts.
var assignmentRetryDelays = []time.Duration{2 * time.Second, 4 * time.Second}

// syncRiderAssignmentAsync runs the acceptance-time sync off the request
// path. If every attempt fails, the next status update retries synchronously.
func (s *DeliveryService) syncRiderAssignmentAsync(orderID int, riderID string, assignedAt time.Time) {
	if s.restaurantCli == nil {
		// Nothing to retry; report it here rather than from a goroutine.
		_ = s.syncRiderAssignment(context.Background(), orderID, riderID, assignedAt, "accept")
		return
	}
	delays := assignmentRetryDelays
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		for attempt := 0; ; attempt++ {
			err := s.syncRiderAssignment(context.Background(), orderID, riderID, assignedAt, "accept")
			if err == nil || !retryableCallback(err) || attempt >= len(delays) {
				return
			}
			time.Sleep(delays[attempt])
		}
	}()
}

// assignmentPayload builds what the customer may see about the rider. A
// profile lookup failure still records the assignment, just without details.
func (s *DeliveryService) assignmentPayload(ctx context.Context, riderID string, assignedAt time.Time) client.AssignRiderPayload {
	payload := client.AssignRiderPayload{
		RiderID:    riderID,
		RiderName:  "Delivery partner",
		AssignedAt: assignedAt.UTC().Format(time.RFC3339),
	}
	rider, err := s.riderRepo.GetByID(ctx, riderID)
	if err != nil || rider == nil {
		return payload
	}
	name := strings.TrimSpace(strings.Join([]string{deref(rider.FirstName), deref(rider.LastName)}, " "))
	if name == "" {
		name = strings.TrimSpace(deref(rider.DisplayName))
	}
	if name != "" {
		payload.RiderName = name
	}
	payload.RiderPhone = deref(rider.Phone)
	payload.VehicleType = deref(rider.VehicleType)
	payload.VehicleNumber = deref(rider.VehicleRegistrationNumber)
	return payload
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func emitAssignmentSyncFailed(orderID int, riderID, trigger string, err error) {
	dispatchtrace.Emit(dispatchtrace.EventAssignmentSyncFailed, dispatchtrace.Fields{
		"order_id":    orderID,
		"rider_id":    riderID,
		"trigger":     trigger,
		"http_status": callbackStatus(err),
		"reason_code": dispatchtrace.ReasonAssignmentSyncFailed,
		"error":       callbackReason(err),
	})
}

// callbackStatus is the HTTP status of a refused callback, 0 for transport errors.
func callbackStatus(err error) int {
	var cbErr *client.CallbackError
	if errors.As(err, &cbErr) {
		return cbErr.StatusCode
	}
	return 0
}

// callbackReason is restaurant-service's own refusal message, or the error.
func callbackReason(err error) string {
	var cbErr *client.CallbackError
	if errors.As(err, &cbErr) && cbErr.Message != "" {
		return cbErr.Message
	}
	return dispatchtrace.ErrorText(err)
}

// retryableCallback: transport failures and 5xx. A 4xx will not change on retry.
func retryableCallback(err error) bool {
	var cbErr *client.CallbackError
	if errors.As(err, &cbErr) {
		return cbErr.StatusCode >= 500
	}
	return true
}

// restaurantTransitionError maps a refused canonical transition.
func restaurantTransitionError(order *models.DeliveryOrder, riderID, newStatus string, err error) error {
	reason := dispatchtrace.ReasonRestaurantRejected
	if !errors.As(err, new(*client.CallbackError)) {
		reason = dispatchtrace.ReasonRestaurantUnavailable
	}
	rejectErr := rejectStatus(order.OrderID, riderID, order.DeliveryStatus, newStatus,
		ErrCodeRestaurantSync, reason,
		fmt.Sprintf("canonical order transition failed: %v", err),
		dispatchtrace.Fields{"http_status": callbackStatus(err), "error": callbackReason(err)})
	return rejectErr
}

// activeDeliveryFields summarises what the rider app received, without values.
func activeDeliveryFields(order *models.DeliveryOrder, snap *repository.OrderRiderSnapshot) dispatchtrace.Fields {
	nav := "complete"
	switch {
	case order.PickupLatitude == 0 && order.PickupLongitude == 0 && strings.TrimSpace(order.PickupAddress) == "":
		nav = "missing_pickup"
	case order.DropLatitude == 0 && order.DropLongitude == 0 && strings.TrimSpace(order.DropAddress) == "":
		nav = "missing_drop"
	}
	fields := dispatchtrace.Fields{
		"order_id":             order.OrderID,
		"delivery_status":      order.DeliveryStatus,
		"navigation_data":      nav,
		"has_pickup_coords":    order.PickupLatitude != 0 || order.PickupLongitude != 0,
		"has_drop_coords":      order.DropLatitude != 0 || order.DropLongitude != 0,
		"has_restaurant_phone": strings.TrimSpace(order.RestaurantPhone) != "",
		"has_customer_phone":   strings.TrimSpace(order.CustomerPhone) != "",
	}
	if snap != nil {
		fields["restaurant_status"] = snap.OrderStatus
		fields["rider_on_order"] = snap.HasRider(deref(order.AssignedRiderID)) || snap.HasRider(deref(order.RiderUserID))
	}
	return fields
}

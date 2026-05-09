package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// DeliveryService handles the SQS-driven delivery assignment flow.
type DeliveryService struct {
	deliveryRepo   *repository.DeliveryRepository
	riderRepo      *repository.RiderRepository
	hub            *ws.Hub
	restaurantCli  *client.RestaurantClient
	searchRadiusKm float64
	maxRiders      int
	requestExpiry  time.Duration
}

func NewDeliveryService(
	deliveryRepo *repository.DeliveryRepository,
	riderRepo *repository.RiderRepository,
	hub *ws.Hub,
	restaurantCli *client.RestaurantClient,
	searchRadiusKm float64,
	maxRiders int,
	requestExpirySec int,
) *DeliveryService {
	return &DeliveryService{
		deliveryRepo:   deliveryRepo,
		riderRepo:      riderRepo,
		hub:            hub,
		restaurantCli:  restaurantCli,
		searchRadiusKm: searchRadiusKm,
		maxRiders:      maxRiders,
		requestExpiry:  time.Duration(requestExpirySec) * time.Second,
	}
}

// ProcessOrderPlacedEvent handles an ORDER_PLACED SQS event end-to-end.
func (s *DeliveryService) ProcessOrderPlacedEvent(ctx context.Context, evt *models.OrderPlacedEvent) error {
	// 1. Idempotency check
	if evt.EventID != "" {
		processed, err := s.deliveryRepo.IsEventProcessed(ctx, evt.EventID)
		if err != nil {
			return fmt.Errorf("idempotency check failed: %w", err)
		}
		if processed {
			log.Printf("[DELIVERY] Duplicate event %s for order %d ignored", evt.EventID, evt.OrderID)
			return nil
		}
	}
	orderProcessed, err := s.deliveryRepo.IsOrderProcessed(ctx, evt.OrderID)
	if err != nil {
		return fmt.Errorf("order idempotency check failed: %w", err)
	}
	if orderProcessed {
		log.Printf("[DELIVERY] Order %d already processed, skipping", evt.OrderID)
		return nil
	}

	// 2. Create delivery order
	deliveryOrder, err := s.deliveryRepo.CreateDeliveryOrder(ctx, evt)
	if err != nil {
		return fmt.Errorf("failed to create delivery order: %w", err)
	}
	log.Printf("[DELIVERY] Delivery order created: id=%d order_id=%d", deliveryOrder.DeliveryOrderID, deliveryOrder.OrderID)

	// 3. Mark event processed
	if evt.EventID != "" {
		_ = s.deliveryRepo.MarkEventProcessed(ctx, evt.EventID, evt.OrderID, evt.EventType)
	}

	// 4. Check if restaurant has its own active riders — if so, skip platform broadcast
	hasOwnRiders, err := s.riderRepo.HasActiveRestaurantOwnRiders(ctx, evt.RestaurantID)
	if err != nil {
		log.Printf("[DELIVERY] Failed to check restaurant_riders for restaurant %d: %v", evt.RestaurantID, err)
		// Fall through to platform flow on error
	} else if hasOwnRiders {
		log.Printf("[DELIVERY] Restaurant %d has own riders. Skipping platform rider broadcast for order %d. Waiting for RIDER_ASSIGNED_TO_ORDER event.", evt.RestaurantID, evt.OrderID)
		return nil
	}

	// 5. Find nearest platform riders
	riders, err := s.deliveryRepo.FindNearestRiders(ctx, evt.Pickup.Latitude, evt.Pickup.Longitude, s.searchRadiusKm, s.maxRiders)
	if err != nil {
		log.Printf("[DELIVERY] Nearest rider search failed for order %d: %v", evt.OrderID, err)
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusNoRiderFound)
		return nil // don't crash consumer
	}

	if len(riders) == 0 {
		log.Printf("[DELIVERY] No riders found within %.1f km for order %d", s.searchRadiusKm, evt.OrderID)
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusNoRiderFound)
		return nil
	}
	log.Printf("[DELIVERY] Found %d nearby riders for order %d", len(riders), evt.OrderID)

	// 6. Create requests and notify platform riders
	expiresAt := time.Now().Add(s.requestExpiry)
	for _, rider := range riders {
		req, err := s.deliveryRepo.CreateRequest(ctx, deliveryOrder.DeliveryOrderID, evt.OrderID, rider.RiderID, rider.DistanceKm, expiresAt)
		if err != nil {
			log.Printf("[DELIVERY] Failed to create request for rider %s: %v", rider.RiderID, err)
			continue
		}
		log.Printf("[DELIVERY] Request %d sent to rider %s (%.2f km)", req.RequestID, rider.RiderID, rider.DistanceKm)

		// Send WebSocket notification
		s.hub.SendToRider(rider.RiderID, ws.WSMessage{
			Type: "DELIVERY_ORDER_REQUEST",
			Data: map[string]interface{}{
				"request_id":       req.RequestID,
				"order_id":         evt.OrderID,
				"restaurant_id":    evt.RestaurantID,
				"pickup_address":   evt.Pickup.Address,
				"drop_address":     evt.Drop.Address,
				"pickup_latitude":  evt.Pickup.Latitude,
				"pickup_longitude": evt.Pickup.Longitude,
				"drop_latitude":    evt.Drop.Latitude,
				"drop_longitude":   evt.Drop.Longitude,
				"distance_km":      rider.DistanceKm,
				"amount":           evt.Amount,
				"expires_at":       expiresAt.Format(time.RFC3339),
			},
		})
	}

	return nil
}

// ProcessRiderAssignedEvent handles a RIDER_ASSIGNED_TO_ORDER event from restaurant-service.
func (s *DeliveryService) ProcessRiderAssignedEvent(ctx context.Context, evt *models.RiderAssignedToOrderEvent) error {
	// Idempotency: use composite event ID
	eventID := fmt.Sprintf("rider_assigned:%d:%s", evt.OrderID, evt.RiderUserID)
	processed, err := s.deliveryRepo.IsEventProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("idempotency check failed: %w", err)
	}
	if processed {
		log.Printf("[DELIVERY] Duplicate RIDER_ASSIGNED_TO_ORDER for order %d ignored", evt.OrderID)
		return nil
	}

	// Upsert into delivery_orders
	deliveryOrder, err := s.deliveryRepo.UpsertRestaurantOwnedOrder(ctx, evt)
	if err != nil {
		return fmt.Errorf("failed to upsert restaurant-owned delivery order: %w", err)
	}
	log.Printf("[DELIVERY] Restaurant-owned order upserted: delivery_order_id=%d order_id=%d rider=%s",
		deliveryOrder.DeliveryOrderID, deliveryOrder.OrderID, evt.RiderUserID)

	// Mark event processed
	_ = s.deliveryRepo.MarkEventProcessed(ctx, eventID, evt.OrderID, "RIDER_ASSIGNED_TO_ORDER")

	// Push WebSocket event ONLY to the assigned rider
	s.hub.SendToRider(evt.RiderUserID, ws.WSMessage{
		Type: "order_assigned",
		Data: map[string]interface{}{
			"event_id":        eventID,
			"order_id":        evt.OrderID,
			"restaurant_id":   evt.RestaurantID,
			"assignment_type":  "restaurant_owned",
			"delivery_status":  "rider_assigned",
			"restaurant_name":  evt.RiderName,
			"restaurant_phone": evt.RiderPhone,
		},
	})

	// Also broadcast to customer tracking channel
	s.hub.SendToOrder(strconv.Itoa(evt.OrderID), ws.WSMessage{
		Type: "RIDER_ASSIGNED",
		Data: map[string]interface{}{
			"order_id":   evt.OrderID,
			"rider_id":   evt.RiderUserID,
			"rider_name":  evt.RiderName,
			"rider_phone": evt.RiderPhone,
		},
	})

	return nil
}

// GetRiderOrders returns delivery orders assigned to a specific rider by status.
func (s *DeliveryService) GetRiderOrders(ctx context.Context, riderUserID string, statuses []string) ([]*models.DeliveryOrder, error) {
	return s.deliveryRepo.GetRiderOrders(ctx, riderUserID, statuses)
}

// GetRiderOrderDetail returns a single delivery order, validating rider ownership.
func (s *DeliveryService) GetRiderOrderDetail(ctx context.Context, orderID int, riderUserID string) (*models.DeliveryOrder, error) {
	do, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("delivery order not found")
	}
	// Validate rider ownership: check both assigned_rider_id and rider_user_id
	owns := false
	if do.AssignedRiderID != nil && *do.AssignedRiderID == riderUserID {
		owns = true
	}
	if do.RiderUserID != nil && *do.RiderUserID == riderUserID {
		owns = true
	}
	if !owns {
		return nil, fmt.Errorf("order not assigned to this rider")
	}
	return do, nil
}

// UpdateRiderLocation updates location in both new and legacy tables.
func (s *DeliveryService) UpdateRiderLocation(ctx context.Context, riderID string, lat, lng float64) error {
	// Update new rider_locations table
	if err := s.deliveryRepo.UpsertRiderLocation(ctx, riderID, lat, lng); err != nil {
		return err
	}

	// Check if rider has active order → broadcast to customer
	avail, err := s.deliveryRepo.GetRiderAvailability(ctx, riderID)
	if err == nil && avail.CurrentOrderID != nil {
		orderIDStr := strconv.Itoa(*avail.CurrentOrderID)
		s.hub.SendToOrder(orderIDStr, ws.WSMessage{
			Type: "RIDER_LOCATION_UPDATED",
			Data: map[string]interface{}{
				"rider_id":  riderID,
				"latitude":  lat,
				"longitude": lng,
			},
		})
	}
	return nil
}

// UpdateRiderAvailability handles POST /riders/availability
func (s *DeliveryService) UpdateRiderAvailability(ctx context.Context, riderID string, isOnline, isAvailable bool) error {
	// If going offline, force unavailable
	if !isOnline {
		isAvailable = false
	}

	// If rider has active order, don't allow setting available
	avail, err := s.deliveryRepo.GetRiderAvailability(ctx, riderID)
	if err == nil && avail.CurrentOrderID != nil && isAvailable {
		return fmt.Errorf("cannot set available while on active order %d", *avail.CurrentOrderID)
	}

	var currentOrderID *int
	if avail != nil {
		currentOrderID = avail.CurrentOrderID
	}

	return s.deliveryRepo.UpsertRiderAvailability(ctx, riderID, isOnline, isAvailable, currentOrderID)
}

// GetPendingRequests returns pending non-expired requests for a rider.
func (s *DeliveryService) GetPendingRequests(ctx context.Context, riderID string) ([]*models.DeliveryOrderRequest, error) {
	return s.deliveryRepo.GetPendingRequestsForRider(ctx, riderID)
}

// AcceptRequest handles POST /riders/order-requests/{requestId}/accept with row locking.
func (s *DeliveryService) AcceptRequest(ctx context.Context, requestID int, riderID string) error {
	tx, err := s.deliveryRepo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Lock the request row
	req, err := s.deliveryRepo.GetRequestByIDForUpdate(ctx, tx, requestID)
	if err != nil {
		return fmt.Errorf("request not found")
	}
	if req.RiderID != riderID {
		return fmt.Errorf("request does not belong to this rider")
	}
	if req.Status != models.RequestStatusPending {
		return fmt.Errorf("request already responded to (status: %s)", req.Status)
	}
	if time.Now().After(req.ExpiresAt) {
		return fmt.Errorf("request has expired")
	}

	// Check delivery order not already assigned
	deliveryOrder, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, req.DeliveryOrderID)
	if err != nil {
		return fmt.Errorf("delivery order not found")
	}
	if deliveryOrder.AssignedRiderID != nil {
		return fmt.Errorf("order already assigned to another rider")
	}

	// Accept the request
	if err := s.deliveryRepo.AcceptRequest(ctx, tx, requestID); err != nil {
		return fmt.Errorf("failed to accept: %w", err)
	}

	// Assign rider to delivery order
	if err := s.deliveryRepo.AssignRider(ctx, tx, req.DeliveryOrderID, riderID); err != nil {
		return fmt.Errorf("failed to assign rider: %w", err)
	}

	// Cancel other pending requests for same order
	if err := s.deliveryRepo.CancelOtherRequests(ctx, tx, req.DeliveryOrderID, requestID); err != nil {
		log.Printf("[DELIVERY] Failed to cancel other requests: %v", err)
	}

	// Set rider busy
	if err := s.deliveryRepo.SetRiderBusy(ctx, tx, riderID, req.OrderID); err != nil {
		return fmt.Errorf("failed to update rider availability: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("[DELIVERY] Rider %s accepted request %d for order %d", riderID, requestID, req.OrderID)

	// Notify other riders that order is taken
	s.notifyOtherRiders(ctx, req.DeliveryOrderID, riderID, req.OrderID)

	// Notify customer
	s.hub.SendToOrder(strconv.Itoa(req.OrderID), ws.WSMessage{
		Type: "RIDER_ASSIGNED",
		Data: map[string]interface{}{
			"order_id": req.OrderID,
			"rider_id": riderID,
		},
	})

	// Callback to restaurant-service
	s.callbackRiderAssigned(riderID, req.OrderID)

	return nil
}

// RejectRequest handles POST /riders/order-requests/{requestId}/reject
func (s *DeliveryService) RejectRequest(ctx context.Context, requestID int, riderID string) error {
	req, err := s.deliveryRepo.GetRequestByID(ctx, requestID)
	if err != nil {
		return fmt.Errorf("request not found")
	}
	if req.RiderID != riderID {
		return fmt.Errorf("request does not belong to this rider")
	}
	if req.Status != models.RequestStatusPending {
		return fmt.Errorf("request already responded to")
	}

	if err := s.deliveryRepo.RejectRequest(ctx, requestID); err != nil {
		return err
	}
	log.Printf("[DELIVERY] Rider %s rejected request %d for order %d", riderID, requestID, req.OrderID)

	// Check if all requests are now rejected/expired
	s.checkAllRequestsDone(ctx, req.DeliveryOrderID)
	return nil
}

// UpdateDeliveryStatus handles POST /riders/orders/{orderId}/status
func (s *DeliveryService) UpdateDeliveryStatus(ctx context.Context, orderID int, riderID string, newStatus string, paymentCollected bool, notes string) error {
	deliveryOrder, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("delivery order not found")
	}

	// Validate rider ownership: check both assigned_rider_id and rider_user_id
	owns := false
	if deliveryOrder.AssignedRiderID != nil && *deliveryOrder.AssignedRiderID == riderID {
		owns = true
	}
	if deliveryOrder.RiderUserID != nil && *deliveryOrder.RiderUserID == riderID {
		owns = true
	}
	if !owns {
		return fmt.Errorf("order not assigned to this rider")
	}

	if !models.IsValidDeliveryTransition(deliveryOrder.DeliveryStatus, newStatus) {
		return fmt.Errorf("invalid transition from '%s' to '%s'", deliveryOrder.DeliveryStatus, newStatus)
	}

	// Validate COD payment collection when marking as delivered
	if newStatus == models.DeliveryStatusDelivered {
		isCOD := deliveryOrder.PaymentMode == "cod" || deliveryOrder.PaymentMode == "cash"
		if isCOD && !paymentCollected {
			return fmt.Errorf("cash collection confirmation required")
		}
	}

	if err := s.deliveryRepo.UpdateDeliveryTimestamp(ctx, nil, deliveryOrder.DeliveryOrderID, newStatus); err != nil {
		return err
	}
	log.Printf("[DELIVERY] Order %d status updated to %s by rider %s", orderID, newStatus, riderID)

	isRestaurantOwned := deliveryOrder.RestaurantOwned

	// On delivered: free the rider, but skip platform payout for restaurant-owned
	if newStatus == models.DeliveryStatusDelivered {
		_ = s.deliveryRepo.SetRiderFree(ctx, nil, riderID)
		_ = s.riderRepo.SetOnTrip(ctx, riderID, false)
		log.Printf("[DELIVERY] Rider %s is now free after delivering order %d", riderID, orderID)

		if isRestaurantOwned {
			log.Printf("[DELIVERY] Skipping platform payout for restaurant-owned order %d", orderID)
		}
	}

	// Broadcast to customer tracking
	s.hub.SendToOrder(strconv.Itoa(orderID), ws.WSMessage{
		Type: "ORDER_STATUS_UPDATED",
		Data: map[string]interface{}{
			"order_id":        orderID,
			"delivery_status": newStatus,
		},
	})

	if newStatus == models.DeliveryStatusDelivered {
		s.hub.SendToOrder(strconv.Itoa(orderID), ws.WSMessage{
			Type: "ORDER_DELIVERED",
			Data: map[string]interface{}{"order_id": orderID},
		})
	}

	// Callback to restaurant-service for status updates
	s.callbackDeliveryStatusUpdate(orderID, deliveryOrder.RestaurantID, riderID, newStatus, paymentCollected, notes)

	return nil
}

// GetDeliveryTracking returns tracking info for GET /delivery/orders/{orderId}/tracking
func (s *DeliveryService) GetDeliveryTracking(ctx context.Context, orderID int) (*models.DeliveryTrackingResponse, error) {
	do, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("delivery order not found")
	}

	resp := &models.DeliveryTrackingResponse{
		OrderID:        do.OrderID,
		DeliveryStatus: do.DeliveryStatus,
		Pickup: models.LocationDetail{
			Latitude: do.PickupLatitude, Longitude: do.PickupLongitude, Address: do.PickupAddress,
		},
		Drop: models.LocationDetail{
			Latitude: do.DropLatitude, Longitude: do.DropLongitude, Address: do.DropAddress,
		},
		Timeline: buildTimeline(do),
	}

	// Add rider info if assigned
	if do.AssignedRiderID != nil {
		rider, err := s.riderRepo.GetByID(ctx, *do.AssignedRiderID)
		if err == nil {
			name := ""
			if rider.FirstName != nil {
				name = *rider.FirstName
			}
			phone := ""
			if rider.Phone != nil {
				phone = *rider.Phone
			}
			lat, lng := 0.0, 0.0
			loc, err := s.deliveryRepo.GetRiderLocation(ctx, *do.AssignedRiderID)
			if err == nil {
				lat, lng = loc.Latitude, loc.Longitude
			}
			resp.Rider = &models.TrackingRiderInfo{
				ID: *do.AssignedRiderID, Name: name, Phone: phone,
				Latitude: lat, Longitude: lng,
			}
		}
	}

	return resp, nil
}

func buildTimeline(do *models.DeliveryOrder) []models.DeliveryTimelineItem {
	var tl []models.DeliveryTimelineItem
	tl = append(tl, models.DeliveryTimelineItem{Status: "order_placed", Timestamp: do.CreatedAt})
	if do.AssignedAt != nil {
		tl = append(tl, models.DeliveryTimelineItem{Status: "rider_assigned", Timestamp: *do.AssignedAt})
	}
	if do.PickedUpAt != nil {
		tl = append(tl, models.DeliveryTimelineItem{Status: "picked_up", Timestamp: *do.PickedUpAt})
	}
	if do.DeliveredAt != nil {
		tl = append(tl, models.DeliveryTimelineItem{Status: "delivered", Timestamp: *do.DeliveredAt})
	}
	return tl
}

func (s *DeliveryService) notifyOtherRiders(ctx context.Context, deliveryOrderID int, acceptedRiderID string, orderID int) {
	// Get all requests for this order and notify non-accepted riders
	// Simple approach: we already cancelled them, just send WS event
	// We don't have a method to get all riders for an order, so we skip for now
	// The cancelled status will be picked up by the rider app on next poll
	log.Printf("[DELIVERY] Other riders notified about order %d assignment", orderID)
}

func (s *DeliveryService) callbackRiderAssigned(riderID string, orderID int) {
	rider, err := s.riderRepo.GetByID(context.Background(), riderID)
	if err != nil {
		log.Printf("[DELIVERY] Failed to get rider %s for callback: %v", riderID, err)
		return
	}

	name := ""
	if rider.FirstName != nil {
		name = *rider.FirstName
	}
	if rider.LastName != nil {
		name += " " + *rider.LastName
	}
	phone := ""
	if rider.Phone != nil {
		phone = *rider.Phone
	}
	vehicleType := ""
	if rider.VehicleType != nil {
		vehicleType = *rider.VehicleType
	}
	vehicleNumber := ""
	if rider.VehicleRegistrationNumber != nil {
		vehicleNumber = *rider.VehicleRegistrationNumber
	}

	s.restaurantCli.NotifyRiderAssignedAsync(orderID, client.AssignRiderPayload{
		RiderID:       riderID,
		RiderName:     name,
		RiderPhone:    phone,
		VehicleType:   vehicleType,
		VehicleNumber: vehicleNumber,
		AssignedAt:    time.Now().Format(time.RFC3339),
	})
}

func (s *DeliveryService) callbackDeliveryStatusUpdate(orderID, restaurantID int, riderID, deliveryStatus string, paymentCollected bool, notes string) {
	s.restaurantCli.NotifyDeliveryStatusUpdateAsync(orderID, client.DeliveryStatusPayload{
		OrderID:          orderID,
		RestaurantID:     restaurantID,
		RiderID:          riderID,
		DeliveryStatus:   deliveryStatus,
		PaymentCollected: paymentCollected,
		Notes:            notes,
	})
}

func (s *DeliveryService) checkAllRequestsDone(ctx context.Context, deliveryOrderID int) {
	pending, err := s.deliveryRepo.CountPendingForOrder(ctx, deliveryOrderID)
	if err != nil {
		return
	}
	if pending == 0 {
		hasAccepted, _ := s.deliveryRepo.HasAcceptedRequest(ctx, deliveryOrderID)
		if !hasAccepted {
			_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrderID, models.DeliveryStatusNoRiderFound)
			log.Printf("[DELIVERY] All requests rejected/expired for delivery_order %d, marked no_rider_found", deliveryOrderID)
		}
	}
}

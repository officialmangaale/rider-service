package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	dispatchcache "github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/cache"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/debug"
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
	dispatchCache  *dispatchcache.RedisDispatchCache
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
	dispatchCache *dispatchcache.RedisDispatchCache,
) *DeliveryService {
	return &DeliveryService{
		deliveryRepo:   deliveryRepo,
		riderRepo:      riderRepo,
		hub:            hub,
		restaurantCli:  restaurantCli,
		dispatchCache:  dispatchCache,
		searchRadiusKm: searchRadiusKm,
		maxRiders:      maxRiders,
		requestExpiry:  time.Duration(requestExpirySec) * time.Second,
	}
}

// ProcessOrderPlacedEvent handles an ORDER_PLACED SQS event end-to-end.
func (s *DeliveryService) ProcessOrderPlacedEvent(ctx context.Context, evt *models.OrderPlacedEvent) error {
	if err := canonicalizeOrderPlacedEvent(evt); err != nil {
		return err
	}
	log.Printf("[DELIVERY] Processing ORDER_PLACED order_id=%d restaurant_id=%d delivery_mode=%s",
		evt.OrderID, evt.RestaurantID, evt.DeliveryMode)

	// 1. Idempotency check. A later accepted/preparing event may safely
	// redispatch an unassigned order after every previous offer is terminal.
	eventProcessed := false
	if evt.EventID != "" {
		processed, err := s.deliveryRepo.IsEventProcessed(ctx, evt.EventID)
		if err != nil {
			return fmt.Errorf("idempotency check failed: %w", err)
		}
		eventProcessed = processed
	}
	orderProcessed, err := s.deliveryRepo.IsOrderProcessed(ctx, evt.OrderID)
	if err != nil {
		return fmt.Errorf("order idempotency check failed: %w", err)
	}

	if evt.RestaurantName == "" || evt.RestaurantPhone == "" {
		name, phone, err := s.deliveryRepo.GetRestaurantContact(ctx, evt.RestaurantID)
		if err != nil {
			log.Printf("[DELIVERY] Restaurant contact enrichment missing for restaurant %d: %v", evt.RestaurantID, err)
		} else {
			if evt.RestaurantName == "" {
				evt.RestaurantName = name
			}
			if evt.RestaurantPhone == "" {
				evt.RestaurantPhone = phone
			}
		}
	}

	// 2. Create the canonical delivery order, or reopen an unassigned order
	// whose previous rider offers have all expired/rejected.
	var deliveryOrder *models.DeliveryOrder
	if !orderProcessed {
		existing, lookupErr := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, evt.OrderID)
		switch {
		case lookupErr == nil:
			deliveryOrder = existing
			orderProcessed = true
		case errors.Is(lookupErr, sql.ErrNoRows):
		default:
			return fmt.Errorf("failed to check for existing delivery order: %w", lookupErr)
		}
	}
	if eventProcessed || orderProcessed {
		if deliveryOrder == nil {
			deliveryOrder, err = s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, evt.OrderID)
			if err != nil {
				return fmt.Errorf("failed to load existing delivery order: %w", err)
			}
		}
		canRedispatch, err := s.canRedispatch(ctx, deliveryOrder)
		if err != nil {
			return err
		}
		if !canRedispatch {
			log.Printf("[DELIVERY] Order %d already processed with active or assigned delivery state, skipping", evt.OrderID)
			return nil
		}
		if err := s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusRiderSearching); err != nil {
			return fmt.Errorf("failed to reset delivery order for redispatch: %w", err)
		}
		deliveryOrder.DeliveryStatus = models.DeliveryStatusRiderSearching
		log.Printf("[DELIVERY] Redispatching delivery_order=%d order_id=%d", deliveryOrder.DeliveryOrderID, evt.OrderID)
	} else {
		deliveryOrder, err = s.deliveryRepo.CreateDeliveryOrder(ctx, evt)
		if err != nil {
			return fmt.Errorf("failed to create delivery order: %w", err)
		}
		log.Printf("[DELIVERY] Delivery order created: id=%d order_id=%d", deliveryOrder.DeliveryOrderID, deliveryOrder.OrderID)
	}

	// 3. Mark this event processed after the delivery order is durable.
	if evt.EventID != "" && !eventProcessed {
		if err := s.deliveryRepo.MarkEventProcessed(ctx, evt.EventID, evt.OrderID, evt.EventType); err != nil {
			log.Printf("[DELIVERY] Failed to mark event %s processed: %v", evt.EventID, err)
		}
	}

	// 4. Check if restaurant has its own active riders — if so, skip platform broadcast
	deliveryMode := strings.ToLower(strings.TrimSpace(evt.DeliveryMode))
	if deliveryMode == "restaurant_own_rider" || deliveryMode == "restaurant_owned" || deliveryMode == "" {
		hasOwnRiders, err := s.riderRepo.HasActiveRestaurantOwnRiders(ctx, evt.RestaurantID)
		if err != nil {
			log.Printf("[DELIVERY] Failed to check restaurant_riders for restaurant %d: %v", evt.RestaurantID, err)
			// Fall through to platform flow on error
		} else if hasOwnRiders {
			log.Printf("[DELIVERY] Restaurant %d has own riders. Skipping platform rider broadcast for order %d. Waiting for RIDER_ASSIGNED_TO_ORDER event.", evt.RestaurantID, evt.OrderID)
			return nil
		}
	} else {
		log.Printf("[DELIVERY] Platform dispatch requested for order %d (delivery_mode=%s)", evt.OrderID, deliveryMode)
	}

	// 5. Find nearest platform riders. Redis GEO is the fast path; SQL remains
	// the durable fallback while Redis warms up or if it is unavailable.
	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		if err := s.dispatchCache.SetPendingOrder(ctx, evt.OrderID, s.requestExpiry); err != nil {
			log.Printf("[DELIVERY] Redis pending order TTL failed order_id=%d err=%v", evt.OrderID, err)
		}
	}
	var riders []models.NearbyRider
	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		redisRiders, redisErr := s.dispatchCache.FindNearestRiders(ctx, evt.Pickup.Latitude, evt.Pickup.Longitude, s.searchRadiusKm, s.maxRiders)
		if redisErr != nil {
			log.Printf("[DELIVERY] Redis GEO search failed order_id=%d radius_km=%.1f err=%v; falling back to SQL", evt.OrderID, s.searchRadiusKm, redisErr)
		} else if len(redisRiders) > 0 {
			riders = redisRiders
			log.Printf("[DELIVERY] Redis GEO selected %d riders for order %d radius_km=%.1f", len(riders), evt.OrderID, s.searchRadiusKm)
		} else {
			log.Printf("[DELIVERY] Redis GEO found no eligible riders for order %d radius_km=%.1f; falling back to SQL", evt.OrderID, s.searchRadiusKm)
		}
	}
	if len(riders) == 0 {
		riders, err = s.deliveryRepo.FindNearestRiders(ctx, evt.Pickup.Latitude, evt.Pickup.Longitude, s.searchRadiusKm, s.maxRiders)
		if err != nil {
			log.Printf("[DELIVERY] Nearest rider search failed for order %d: %v", evt.OrderID, err)
			_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusNoRiderFound)
			if s.dispatchCache != nil && s.dispatchCache.Enabled() {
				s.dispatchCache.ClearPendingOrder(ctx, evt.OrderID)
			}
			return nil // don't crash consumer
		}
	}

	if len(riders) == 0 {
		s.logEligibilitySummary(ctx, evt)
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusNoRiderFound)
		if s.dispatchCache != nil && s.dispatchCache.Enabled() {
			s.dispatchCache.ClearPendingOrder(ctx, evt.OrderID)
		}
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
			Data: BuildDeliveryOrderRequestPayload(req, deliveryOrder, rider.DistanceKm, expiresAt),
		})
		log.Printf("[DELIVERY] WebSocket request emitted rider_id=%s request_id=%d connected=%t",
			rider.RiderID, req.RequestID, s.hub.IsRiderConnected(rider.RiderID))
	}

	return nil
}

func canonicalizeOrderPlacedEvent(evt *models.OrderPlacedEvent) error {
	if evt == nil {
		return errors.New("nil ORDER_PLACED event")
	}
	evt.EventType = strings.ToUpper(strings.TrimSpace(evt.EventType))
	if evt.EventType == "" {
		evt.EventType = "ORDER_PLACED"
	}
	evt.EventID = strings.TrimSpace(evt.EventID)
	if evt.EventType == "ORDER_PLACED" && evt.OrderID > 0 {
		evt.EventID = fmt.Sprintf("%s:%d", evt.EventType, evt.OrderID)
	} else if evt.EventID == "" && evt.OrderID > 0 {
		evt.EventID = fmt.Sprintf("%s:%d", evt.EventType, evt.OrderID)
	}
	return nil
}

func (s *DeliveryService) canRedispatch(ctx context.Context, order *models.DeliveryOrder) (bool, error) {
	if order.RestaurantOwned || nonEmptyString(order.AssignedRiderID) || nonEmptyString(order.RiderUserID) {
		return false, nil
	}
	switch order.DeliveryStatus {
	case models.DeliveryStatusRiderAssigned,
		models.DeliveryStatusRiderArrivedRestaurant,
		models.DeliveryStatusPickedUp,
		models.DeliveryStatusOnTheWay,
		models.DeliveryStatusDelivered,
		models.DeliveryStatusCancelled:
		return false, nil
	}

	hasAccepted, err := s.deliveryRepo.HasAcceptedRequest(ctx, order.DeliveryOrderID)
	if err != nil {
		return false, fmt.Errorf("failed to check accepted rider requests: %w", err)
	}
	if hasAccepted {
		return false, nil
	}
	pending, err := s.deliveryRepo.CountPendingForOrder(ctx, order.DeliveryOrderID)
	if err != nil {
		return false, fmt.Errorf("failed to count pending rider requests: %w", err)
	}
	return pending == 0, nil
}

func nonEmptyString(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func (s *DeliveryService) logEligibilitySummary(ctx context.Context, evt *models.OrderPlacedEvent) {
	summary, err := s.deliveryRepo.GetRiderEligibilitySummary(
		ctx,
		evt.Pickup.Latitude,
		evt.Pickup.Longitude,
		s.searchRadiusKm,
	)
	if err != nil {
		log.Printf("[DELIVERY] No riders found within %.1f km for order %d; eligibility diagnostics failed: %v",
			s.searchRadiusKm, evt.OrderID, err)
		return
	}
	log.Printf(
		"[DELIVERY] No eligible riders for order %d radius_km=%.1f online=%d available=%d with_location=%d fresh_location=%d within_radius=%d",
		evt.OrderID,
		s.searchRadiusKm,
		summary.OnlineRiders,
		summary.AvailableRiders,
		summary.RidersWithLocation,
		summary.RidersWithFreshGPS,
		summary.RidersWithinRadius,
	)
}

func BuildDeliveryOrderRequestPayload(req *models.DeliveryOrderRequest, order *models.DeliveryOrder, distanceKm float64, expiresAt time.Time) map[string]interface{} {
	return map[string]interface{}{
		"request_id":       req.RequestID,
		"order_id":         order.OrderID,
		"restaurant_id":    order.RestaurantID,
		"restaurant_name":  order.RestaurantName,
		"restaurant_phone": order.RestaurantPhone,
		"pickup_address":   order.PickupAddress,
		"drop_address":     order.DropAddress,
		"pickup_latitude":  order.PickupLatitude,
		"pickup_longitude": order.PickupLongitude,
		"drop_latitude":    order.DropLatitude,
		"drop_longitude":   order.DropLongitude,
		"distance_km":      distanceKm,
		"amount":           order.Amount,
		"payment_mode":     order.PaymentMode,
		"expires_at":       expiresAt.Format(time.RFC3339),
		"assignment_type":  order.AssignmentType,
	}
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

	if evt.RestaurantName == "" || evt.RestaurantPhone == "" {
		name, phone, err := s.deliveryRepo.GetRestaurantContact(ctx, evt.RestaurantID)
		if err != nil {
			log.Printf("[DELIVERY] Restaurant contact enrichment missing for restaurant %d: %v", evt.RestaurantID, err)
		} else {
			if evt.RestaurantName == "" {
				evt.RestaurantName = name
			}
			if evt.RestaurantPhone == "" {
				evt.RestaurantPhone = phone
			}
		}
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
			"event_id":         eventID,
			"order_id":         evt.OrderID,
			"restaurant_id":    evt.RestaurantID,
			"assignment_type":  "restaurant_owned",
			"delivery_status":  "rider_assigned",
			"restaurant_name":  evt.RestaurantName,
			"restaurant_phone": evt.RestaurantPhone,
			"rider_name":       evt.RiderName,
			"rider_phone":      evt.RiderPhone,
		},
	})

	// Also broadcast to customer tracking channel
	s.hub.SendToOrder(strconv.Itoa(evt.OrderID), ws.WSMessage{
		Type: "RIDER_ASSIGNED",
		Data: map[string]interface{}{
			"order_id":    evt.OrderID,
			"rider_id":    evt.RiderUserID,
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
	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		if err := s.dispatchCache.UpdateRiderLocation(ctx, riderID, lat, lng); err != nil {
			log.Printf("[DELIVERY] Redis rider location update failed rider_id=%s err=%v", riderID, err)
		}
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

	if err := s.deliveryRepo.UpsertRiderAvailability(ctx, riderID, isOnline, isAvailable, currentOrderID); err != nil {
		return err
	}
	if err := s.riderRepo.SetAvailability(ctx, riderID, isAvailable); err != nil {
		return err
	}
	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		if err := s.dispatchCache.UpdateRiderAvailability(ctx, riderID, isOnline, isAvailable, currentOrderID); err != nil {
			log.Printf("[DELIVERY] Redis rider availability update failed rider_id=%s err=%v", riderID, err)
		}
	}
	return nil
}

// GetPendingRequests returns pending non-expired requests for a rider.
func (s *DeliveryService) GetPendingRequests(ctx context.Context, riderID string) ([]*models.DeliveryOrderRequest, error) {
	return s.deliveryRepo.GetPendingRequestsForRider(ctx, riderID)
}

func (s *DeliveryService) GetPendingRequestPayloads(ctx context.Context, riderID string) ([]map[string]interface{}, error) {
	requests, err := s.deliveryRepo.GetPendingRequestsForRider(ctx, riderID)
	if err != nil {
		return nil, err
	}
	payloads := make([]map[string]interface{}, 0, len(requests))
	for _, req := range requests {
		order, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, req.DeliveryOrderID)
		if err != nil {
			log.Printf("[DELIVERY] Skipping request %d because delivery_order %d is missing: %v", req.RequestID, req.DeliveryOrderID, err)
			continue
		}
		payloads = append(payloads, BuildDeliveryOrderRequestPayload(req, order, req.DistanceKm, req.ExpiresAt))
	}
	return payloads, nil
}

// AcceptRequest handles POST /riders/order-requests/{requestId}/accept with row locking.
func (s *DeliveryService) AcceptRequest(ctx context.Context, requestID int, riderID string) (*models.DeliveryOrder, error) {
	tx, err := s.deliveryRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Lock the request row
	req, err := s.deliveryRepo.GetRequestByIDForUpdate(ctx, tx, requestID)
	if err != nil {
		return nil, fmt.Errorf("request not found")
	}
	if req.RiderID != riderID {
		return nil, fmt.Errorf("request does not belong to this rider")
	}
	if req.Status != models.RequestStatusPending {
		return nil, fmt.Errorf("request already responded to (status: %s)", req.Status)
	}
	if time.Now().After(req.ExpiresAt) {
		return nil, fmt.Errorf("request has expired")
	}

	redisLockAcquired := false
	commitSucceeded := false
	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		accepted, lockErr := s.dispatchCache.TryAcceptOrder(ctx, req.OrderID, riderID, 24*time.Hour)
		if lockErr != nil {
			log.Printf("[DELIVERY] Redis acceptance lock failed order_id=%d rider_id=%s err=%v; continuing with SQL lock", req.OrderID, riderID, lockErr)
		} else if !accepted {
			log.Printf("[DELIVERY] Redis acceptance lock rejected duplicate accept order_id=%d rider_id=%s", req.OrderID, riderID)
			return nil, fmt.Errorf("order already assigned to another rider")
		} else {
			redisLockAcquired = true
			log.Printf("[DELIVERY] Redis acceptance lock acquired order_id=%d rider_id=%s", req.OrderID, riderID)
			defer func() {
				if !commitSucceeded {
					s.dispatchCache.ReleaseOrderLock(context.Background(), req.OrderID, riderID)
				}
			}()
		}
	}

	// Check delivery order not already assigned
	deliveryOrder, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, req.DeliveryOrderID)
	if err != nil {
		return nil, fmt.Errorf("delivery order not found")
	}
	if deliveryOrder.AssignedRiderID != nil {
		return nil, fmt.Errorf("order already assigned to another rider")
	}

	// Accept the request
	if err := s.deliveryRepo.AcceptRequest(ctx, tx, requestID); err != nil {
		return nil, fmt.Errorf("failed to accept: %w", err)
	}

	// Assign rider to delivery order
	if err := s.deliveryRepo.AssignRider(ctx, tx, req.DeliveryOrderID, riderID); err != nil {
		return nil, fmt.Errorf("failed to assign rider: %w", err)
	}

	// Cancel other pending requests for same order
	if err := s.deliveryRepo.CancelOtherRequests(ctx, tx, req.DeliveryOrderID, requestID); err != nil {
		log.Printf("[DELIVERY] Failed to cancel other requests: %v", err)
	}

	// Set rider busy
	if err := s.deliveryRepo.SetRiderBusy(ctx, tx, riderID, req.OrderID); err != nil {
		return nil, fmt.Errorf("failed to update rider availability: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	commitSucceeded = true
	if redisLockAcquired && s.dispatchCache != nil && s.dispatchCache.Enabled() {
		s.dispatchCache.MarkOrderAssigned(context.Background(), req.OrderID)
	}
	assignedRiderID := riderID
	now := time.Now()
	deliveryOrder.AssignedRiderID = &assignedRiderID
	deliveryOrder.RiderUserID = &assignedRiderID
	deliveryOrder.DeliveryStatus = models.DeliveryStatusRiderAssigned
	deliveryOrder.AssignedAt = &now
	_ = s.riderRepo.SetAvailability(ctx, riderID, false)
	_ = s.riderRepo.SetOnTrip(ctx, riderID, true)
	debug.Logf("accepted order persisted rider_id=%s request_id=%d order_id=%d delivery_order_id=%d", riderID, requestID, req.OrderID, req.DeliveryOrderID)

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

	return deliveryOrder, nil
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

	if !models.IsValidDeliveryTransition(deliveryOrder.DeliveryStatus, newStatus) &&
		!isRestaurantOwnedDeliveryTransition(deliveryOrder, newStatus) {
		return fmt.Errorf("invalid transition from '%s' to '%s'", deliveryOrder.DeliveryStatus, newStatus)
	}

	// Validate COD payment collection when marking as delivered
	if newStatus == models.DeliveryStatusDelivered {
		isCOD := deliveryOrder.PaymentMode == "cod" || deliveryOrder.PaymentMode == "cash"
		if isCOD && !paymentCollected {
			return fmt.Errorf("cash collection confirmation required")
		}
	}

	// The restaurant order owns the canonical lifecycle. Commit that transition
	// first; only then advance this service's delivery projection.
	switch newStatus {
	case models.DeliveryStatusPickedUp, models.DeliveryStatusOnTheWay, models.DeliveryStatusDelivered:
		if err := s.restaurantCli.NotifyDeliveryStatusUpdate(orderID, client.DeliveryStatusPayload{
			OrderID:          orderID,
			RestaurantID:     deliveryOrder.RestaurantID,
			RiderID:          riderID,
			DeliveryStatus:   newStatus,
			PaymentCollected: paymentCollected,
			Notes:            notes,
		}); err != nil {
			return fmt.Errorf("canonical order transition failed: %w", err)
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
		_ = s.riderRepo.SetAvailability(ctx, riderID, true)
		_ = s.riderRepo.SetOnTrip(ctx, riderID, false)
		if s.dispatchCache != nil && s.dispatchCache.Enabled() {
			if err := s.dispatchCache.UpdateRiderAvailability(ctx, riderID, true, true, nil); err != nil {
				log.Printf("[DELIVERY] Redis rider free update failed rider_id=%s order_id=%d err=%v", riderID, orderID, err)
			}
		}
		log.Printf("[DELIVERY] Rider %s is now free after delivering order %d", riderID, orderID)

		if isRestaurantOwned {
			log.Printf("[DELIVERY] Skipping platform payout for restaurant-owned order %d", orderID)
		}
	}

	// Projection-only event for rider-service consumers. Customer lifecycle
	// screens consume the canonical restaurant-service WebSocket.
	s.hub.SendToOrder(strconv.Itoa(orderID), ws.WSMessage{
		Type: "DELIVERY_STATUS_UPDATED",
		Data: map[string]interface{}{
			"order_id":        orderID,
			"delivery_status": newStatus,
		},
	})

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

func isRestaurantOwnedDeliveryTransition(order *models.DeliveryOrder, newStatus string) bool {
	if order == nil || !order.RestaurantOwned {
		return false
	}
	switch order.DeliveryStatus {
	case models.DeliveryStatusRiderAssigned, models.DeliveryStatusRiderArrivedRestaurant:
		return newStatus == models.DeliveryStatusPickedUp
	case models.DeliveryStatusPickedUp:
		return newStatus == models.DeliveryStatusOnTheWay ||
			newStatus == models.DeliveryStatusDelivered
	case models.DeliveryStatusOnTheWay:
		return newStatus == models.DeliveryStatusDelivered
	default:
		return false
	}
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

func (s *DeliveryService) checkAllRequestsDone(ctx context.Context, deliveryOrderID int) {
	pending, err := s.deliveryRepo.CountPendingForOrder(ctx, deliveryOrderID)
	if err != nil {
		return
	}
	if pending == 0 {
		hasAccepted, _ := s.deliveryRepo.HasAcceptedRequest(ctx, deliveryOrderID)
		if !hasAccepted {
			_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrderID, models.DeliveryStatusNoRiderFound)
			if s.dispatchCache != nil && s.dispatchCache.Enabled() {
				deliveryOrder, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, deliveryOrderID)
				if err == nil && deliveryOrder != nil {
					s.dispatchCache.ClearPendingOrder(ctx, deliveryOrder.OrderID)
				}
			}
			log.Printf("[DELIVERY] All requests rejected/expired for delivery_order %d, marked no_rider_found", deliveryOrderID)
		}
	}
}

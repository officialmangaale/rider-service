package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	dispatchcache "github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/cache"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/debug"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
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

	// trace is the opt-in per-target diagnostic mode (dispatchtrace.Trace).
	// Zero value: off.
	trace dispatchtrace.Trace

	// riderReferralEnabled gates the referral callback on delivery completion.
	// False by default, so a deployment that has not opted in behaves exactly
	// as it did before the referral programme existed.
	riderReferralEnabled bool

	// background tracks fire-and-forget work (acceptance-time assignment
	// sync) so tests can wait for it; production never waits.
	background sync.WaitGroup
}

// SetRiderReferralEnabled turns the rider-referral qualification callback on.
//
// A setter rather than a constructor argument so that every existing caller
// keeps compiling unchanged and the default stays off.
func (s *DeliveryService) SetRiderReferralEnabled(enabled bool) {
	s.riderReferralEnabled = enabled
}

// SetTrace enables the per-target dispatch trace. Tracing is observation
// only: it never changes which riders are offered an order.
func (s *DeliveryService) SetTrace(trace dispatchtrace.Trace) {
	s.trace = trace
	if trace.RiderID != "" {
		dispatchtrace.Emit(dispatchtrace.EventTraceConfig, dispatchtrace.Fields{
			"active":   true,
			"order_id": trace.OrderID,
			"rider_id": trace.RiderID,
			"until":    trace.Until.UTC().Format(time.RFC3339),
		})
	}
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
	if models.NormalizeSourceOrderType(evt.SourceOrderType) == models.SourceOrderTypeFood {
		allowed, err := s.deliveryRepo.FoodDispatchAllowed(ctx, evt.OrderID)
		if err != nil {
			return err
		}
		if !allowed {
			reason, diagnosticErr := s.deliveryRepo.FoodDispatchBlockReason(ctx, evt.OrderID)
			if diagnosticErr != nil {
				return fmt.Errorf("dispatch exclusion lookup: %w", diagnosticErr)
			}
			log.Printf("[DELIVERY] Dispatch excluded order_id=%d reason_code=%s", evt.OrderID, reason)
			return nil
		}
		if err := s.deliveryRepo.RefreshFoodEvent(ctx, evt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Printf("[DELIVERY] Dispatch excluded order_id=%d reason_code=pickup_missing_invalid_or_order_no_longer_offerable", evt.OrderID)
				return nil
			}
			return err
		}
		evt.DeliveryMode = "platform"
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
	sourceType := models.NormalizeSourceOrderType(evt.SourceOrderType)
	orderProcessed, err := s.deliveryRepo.IsOrderProcessed(ctx, evt.OrderID, sourceType)
	if err != nil {
		return fmt.Errorf("order idempotency check failed: %w", err)
	}

	// A grocery event names its shop; only a food one is enriched from the
	// restaurants table, which has no row for a grocery merchant.
	if sourceType == models.SourceOrderTypeFood && (evt.RestaurantName == "" || evt.RestaurantPhone == "") {
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
		existing, lookupErr := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, evt.OrderID, sourceType)
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
			deliveryOrder, err = s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, evt.OrderID, sourceType)
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
		if err := s.deliveryRepo.MarkEventProcessed(ctx, evt.EventID, evt.OrderID, evt.EventType, sourceType); err != nil {
			log.Printf("[DELIVERY] Failed to mark event %s processed: %v", evt.EventID, err)
		}
	}

	// 4. Own-rider hold.
	//
	// restaurant-service is the authority on whether an order waits for the
	// restaurant's own riders. It checks that one is actually live, and either
	// withholds the event entirely or sends an explicit delivery_mode=platform.
	// This service therefore dispatches everything it receives, except an
	// explicitly restaurant-owned event, which it re-checks defensively.
	//
	// An EMPTY mode dispatches. It used to trigger the own-rider check too, but
	// that check never ran: its query compared a varchar with a uuid and failed
	// on every call, and the error path fell through to dispatch. So an empty
	// mode has always dispatched in practice — 94% of delivery orders carry
	// one. Making the check succeed for them would have started holding orders
	// at any restaurant with an online own rider, including restaurants whose
	// owners were never expecting to assign one.
	deliveryMode := strings.ToLower(strings.TrimSpace(evt.DeliveryMode))
	if sourceType == models.SourceOrderTypeGrocery {
		// restaurant-service ran the own-rider decision for the shop before
		// publishing, so a grocery event always means "offer this to Mangaale
		// riders". Re-checking here would consult restaurant_riders, which
		// knows nothing about grocery shops.
		log.Printf("[DELIVERY] Grocery dispatch requested for grocery_order_id=%d merchant_id=%d", evt.OrderID, evt.MerchantID)
	} else if requiresOwnRiderCheck(deliveryMode) {
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

	// 5. Find nearest platform riders.
	dispatchtrace.Emit(dispatchtrace.EventAttemptStarted, dispatchtrace.Fields{
		"order_id":          evt.OrderID,
		"delivery_order_id": deliveryOrder.DeliveryOrderID,
		"event_id":          evt.EventID,
		"trigger":           "order_event",
		"delivery_mode":     deliveryMode,
	})
	s.markPendingInCache(ctx, evt.OrderID)
	riders, err := s.findOfferableRiders(ctx, deliveryOrder.DeliveryOrderID, evt.OrderID, evt.Pickup.Latitude, evt.Pickup.Longitude)
	s.traceEligibility(ctx, "order_event", evt.OrderID, deliveryOrder.DeliveryOrderID, evt.Pickup.Latitude, evt.Pickup.Longitude, riders, err)
	if err != nil {
		log.Printf("[DELIVERY] Nearest rider search failed for order %d: %v", evt.OrderID, err)
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusNoRiderFound)
		if s.dispatchCache != nil && s.dispatchCache.Enabled() {
			s.dispatchCache.ClearPendingOrder(ctx, evt.OrderID)
		}
		return nil // don't crash consumer
	}

	if len(riders) == 0 {
		// Not final: RedispatchWorker offers the order again once a rider
		// becomes eligible, for as long as the restaurant still wants one.
		_ = s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, deliveryOrder.DeliveryOrderID, models.DeliveryStatusNoRiderFound)
		s.hub.SendToOrder(strconv.Itoa(evt.OrderID), ws.WSMessage{
			Type: "DELIVERY_STATUS_UPDATED",
			Data: map[string]interface{}{
				"order_id":        evt.OrderID,
				"delivery_status": models.DeliveryStatusNoRiderFound,
			},
		})
		if s.dispatchCache != nil && s.dispatchCache.Enabled() {
			s.dispatchCache.ClearPendingOrder(ctx, evt.OrderID)
		}
		return nil
	}
	log.Printf("[DELIVERY] Found %d nearby riders for order %d", len(riders), evt.OrderID)

	// 6. Create requests and notify platform riders
	s.sendOffers(ctx, deliveryOrder, riders)

	return nil
}

// markPendingInCache records the order as awaiting a rider in Redis. It is
// informational only; the acceptance lock does not depend on it.
func (s *DeliveryService) markPendingInCache(ctx context.Context, orderID int) {
	if s.dispatchCache == nil || !s.dispatchCache.Enabled() {
		return
	}
	if err := s.dispatchCache.SetPendingOrder(ctx, orderID, s.requestExpiry); err != nil {
		log.Printf("[DELIVERY] Redis pending order TTL failed order_id=%d err=%v", orderID, err)
	}
}

// findOfferableRiders returns the nearest eligible platform riders for a
// pickup point, minus any rider who has already declined this delivery order.
//
// Redis GEO is the fast path; SQL remains the durable fallback while Redis
// warms up or if it is unavailable. An error is returned only when the SQL
// search itself fails.
func (s *DeliveryService) findOfferableRiders(ctx context.Context, deliveryOrderID, orderID int, pickupLat, pickupLng float64) ([]models.NearbyRider, error) {
	// A declined rider is dropped after the search, so ask for that many more
	// to keep up to maxRiders offers. A first dispatch has no declines, so it
	// behaves exactly as before.
	declined, err := s.deliveryRepo.DeclinedRiderIDs(ctx, deliveryOrderID)
	if err != nil {
		// Offering to a rider who declined is a nuisance; not offering at all
		// strands the order. Carry on, but say so.
		log.Printf("[DELIVERY] Declined-rider lookup failed order_id=%d err=%v; offering without the exclusion", orderID, err)
		declined = nil
	}
	limit := s.maxRiders + len(declined)

	if s.dispatchCache != nil && s.dispatchCache.Enabled() {
		if riders := s.redisCandidateRiders(ctx, orderID, pickupLat, pickupLng, limit, declined); len(riders) >= s.maxRiders {
			return riders, nil
		}
	}
	// PostgreSQL is the source of truth and the complete search.
	sqlRiders, err := s.deliveryRepo.FindNearestRiders(ctx, pickupLat, pickupLng, s.searchRadiusKm, limit)
	if err != nil {
		return nil, err
	}
	return excludeDeclinedRiders(sqlRiders, declined, s.maxRiders), nil
}

// redisCandidateRiders is the dispatch fast path. Redis GEO proposes nearby
// riders with a fresh location; PostgreSQL decides which of them are
// eligible, using the same rules as the full search. Redis data can therefore
// be stale or partial without ever producing a wrong offer.
//
// Its result is used only when it already fills the offer list. Otherwise
// the caller runs the full SQL search, so a rider missing from Redis (a
// failed cache write, a flushed cache, a fresh deploy) is never skipped.
func (s *DeliveryService) redisCandidateRiders(ctx context.Context, orderID int, pickupLat, pickupLng float64, limit int, declined map[string]bool) []models.NearbyRider {
	fields := dispatchtrace.Fields{"order_id": orderID, "radius_km": s.searchRadiusKm}
	ids, err := s.dispatchCache.NearbyCandidateIDs(ctx, pickupLat, pickupLng, s.searchRadiusKm, limit*3)
	if err != nil {
		fields["result"] = "fallback_sql"
		fields["reason_code"] = dispatchtrace.ReasonRedisUnavailable
		fields["error"] = dispatchtrace.ErrorText(err)
		dispatchtrace.Emit(dispatchtrace.EventRedisSearch, fields)
		return nil
	}
	fields["candidates"] = len(ids)
	if len(ids) == 0 {
		fields["result"] = "fallback_sql"
		fields["reason_code"] = dispatchtrace.ReasonRedisNoCandidates
		dispatchtrace.Emit(dispatchtrace.EventRedisSearch, fields)
		return nil
	}
	verified, err := s.deliveryRepo.FindNearestRidersAmong(ctx, pickupLat, pickupLng, s.searchRadiusKm, limit, ids)
	if err != nil {
		fields["result"] = "fallback_sql"
		fields["reason_code"] = dispatchtrace.ReasonEligibilityQueryFailed
		fields["error"] = dispatchtrace.ErrorText(err)
		dispatchtrace.Emit(dispatchtrace.EventRedisSearch, fields)
		return nil
	}
	riders := excludeDeclinedRiders(verified, declined, s.maxRiders)
	fields["verified"] = len(riders)
	if len(riders) >= s.maxRiders {
		fields["result"] = "used"
	} else {
		fields["result"] = "fallback_sql"
		fields["reason_code"] = dispatchtrace.ReasonRedisTooFewVerified
	}
	dispatchtrace.Emit(dispatchtrace.EventRedisSearch, fields)
	return riders
}

// excludeDeclinedRiders drops riders in declined and caps the result at max,
// keeping the nearest-first order of the input.
func excludeDeclinedRiders(riders []models.NearbyRider, declined map[string]bool, max int) []models.NearbyRider {
	out := make([]models.NearbyRider, 0, len(riders))
	for _, rider := range riders {
		if declined[rider.RiderID] {
			continue
		}
		if max > 0 && len(out) >= max {
			break
		}
		out = append(out, rider)
	}
	return out
}

// sendOffers creates one request per rider and pushes it over the rider's
// socket. The request row is the source of truth: a rider without a live
// socket receives the same offer from GET /riders/order-requests/pending.
//
// It returns how many offers were created. CreateRequest refuses to reopen a
// request that is still pending, so running this twice for the same order
// cannot double-offer a rider.
func (s *DeliveryService) sendOffers(ctx context.Context, deliveryOrder *models.DeliveryOrder, riders []models.NearbyRider) int {
	expiresAt := time.Now().Add(s.requestExpiry)
	offered := 0
	for _, rider := range riders {
		req, err := s.deliveryRepo.CreateRequest(ctx, deliveryOrder.DeliveryOrderID, deliveryOrder.OrderID, rider.RiderID, rider.DistanceKm, expiresAt)
		if err != nil {
			log.Printf("[DELIVERY] Failed to create request for rider %s: %v", rider.RiderID, err)
			reason := dispatchtrace.ReasonOfferPersistFailed
			if errors.Is(err, sql.ErrNoRows) {
				// ON CONFLICT ... WHERE matched nothing: this rider already
				// holds a pending offer for the order.
				reason = dispatchtrace.ReasonOfferNotReopened
			}
			dispatchtrace.Emit(dispatchtrace.EventOfferPersistFailed, dispatchtrace.Fields{
				"order_id":          deliveryOrder.OrderID,
				"delivery_order_id": deliveryOrder.DeliveryOrderID,
				"rider_id":          rider.RiderID,
				"reason_code":       reason,
				"error":             dispatchtrace.ErrorText(err),
			})
			continue
		}
		offered++
		log.Printf("[DELIVERY] Request %d sent to rider %s (%.2f km)", req.RequestID, rider.RiderID, rider.DistanceKm)
		dispatchtrace.Emit(dispatchtrace.EventOfferPersisted, dispatchtrace.Fields{
			"order_id":          deliveryOrder.OrderID,
			"delivery_order_id": deliveryOrder.DeliveryOrderID,
			"request_id":        req.RequestID,
			"rider_id":          rider.RiderID,
			"distance_km":       rider.DistanceKm,
			"expires_in":        time.Until(expiresAt).Round(time.Second),
		})

		// Send WebSocket notification
		connections, enqueued := s.hub.SendToRiderCount(rider.RiderID, ws.WSMessage{
			Type: "DELIVERY_ORDER_REQUEST",
			Data: BuildDeliveryOrderRequestPayload(req, deliveryOrder, rider.DistanceKm, expiresAt),
		})
		log.Printf("[DELIVERY] WebSocket request emitted rider_id=%s request_id=%d connected=%t",
			rider.RiderID, req.RequestID, connections > 0)
		publish := dispatchtrace.Fields{
			"order_id":    deliveryOrder.OrderID,
			"request_id":  req.RequestID,
			"rider_id":    rider.RiderID,
			"connections": connections,
			"enqueued":    enqueued,
			"result":      "enqueued",
		}
		switch {
		case connections == 0:
			// The offer is still delivered by the app's pending-requests poll.
			publish["result"] = "not_sent"
			publish["reason_code"] = dispatchtrace.ReasonNoMatchingConnection
		case enqueued < connections:
			publish["result"] = "partial"
			publish["reason_code"] = dispatchtrace.ReasonSocketBackpressure
		}
		dispatchtrace.Emit(dispatchtrace.EventOfferPublish, publish)
	}
	return offered
}

// RedispatchOrder offers an unmatched platform delivery order to the riders
// who are eligible now. RedispatchWorker calls it for orders selected by
// FindRedispatchCandidates.
//
// The candidate query is re-checked against the current row through
// canRedispatch, because a rider may have accepted, or the restaurant assigned
// its own rider, between the sweep's SELECT and this call.
//
// With no eligible rider it writes nothing and returns 0, so a sweep that
// finds nobody leaves no trace and the next sweep simply tries again.
func (s *DeliveryService) RedispatchOrder(ctx context.Context, deliveryOrderID int) (int, error) {
	order, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, deliveryOrderID)
	if err != nil {
		return 0, fmt.Errorf("load delivery_order %d: %w", deliveryOrderID, err)
	}
	ok, err := s.canRedispatch(ctx, order)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}

	riders, err := s.findOfferableRiders(ctx, order.DeliveryOrderID, order.OrderID, order.PickupLatitude, order.PickupLongitude)
	// A sweep repeats every 20 s, so an unchanged outcome is not re-logged;
	// the worker logs errors, and a found rider or an active trace is logged.
	if len(riders) > 0 || s.trace.Covers(order.OrderID, time.Now()) {
		s.traceEligibility(ctx, "redispatch", order.OrderID, order.DeliveryOrderID, order.PickupLatitude, order.PickupLongitude, riders, err)
	}
	if err != nil {
		return 0, fmt.Errorf("find riders for order %d: %w", order.OrderID, err)
	}
	if len(riders) == 0 {
		return 0, nil
	}

	if err := s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, order.DeliveryOrderID, models.DeliveryStatusRiderSearching); err != nil {
		return 0, fmt.Errorf("reopen delivery_order %d for redispatch: %w", order.DeliveryOrderID, err)
	}
	order.DeliveryStatus = models.DeliveryStatusRiderSearching
	s.markPendingInCache(ctx, order.OrderID)

	offered := s.sendOffers(ctx, order, riders)
	if offered == 0 {
		// Every insert failed. Put the status back so it does not claim a
		// search is running; the failures are already logged per rider.
		if err := s.deliveryRepo.UpdateDeliveryStatus(ctx, nil, order.DeliveryOrderID, models.DeliveryStatusNoRiderFound); err != nil {
			log.Printf("[DELIVERY] Could not restore no_rider_found for delivery_order %d: %v", order.DeliveryOrderID, err)
		}
		return 0, nil
	}
	log.Printf("[DELIVERY] Redispatched order %d to %d rider(s)", order.OrderID, offered)
	return offered, nil
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
	// The id is derived from the order so a redelivered message is recognised
	// whatever id the producer chose. It must also be scoped by order type: a
	// grocery order id and a food order id come from different sequences, and
	// sharing a key would make one of the two look already processed.
	//
	// A food id keeps its exact old shape, so events in flight during a deploy
	// and rows already in processed_events still match.
	if evt.OrderID > 0 && (evt.EventType == "ORDER_PLACED" || evt.EventID == "") {
		if models.NormalizeSourceOrderType(evt.SourceOrderType) == models.SourceOrderTypeGrocery {
			evt.EventID = fmt.Sprintf("%s:%s:%d", evt.EventType, models.SourceOrderTypeGrocery, evt.OrderID)
		} else {
			evt.EventID = fmt.Sprintf("%s:%d", evt.EventType, evt.OrderID)
		}
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

	if !order.IsGrocery() {
		allowed, err := s.deliveryRepo.FoodDispatchAllowed(ctx, order.OrderID)
		if err != nil || !allowed {
			return false, err
		}
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

// traceEligibility records the outcome of one rider search.
//
// A failed search and a search that found nobody both leave the delivery
// order as no_rider_found, so the log is the only place they differ. The
// aggregate funnel (from a separate, simpler query) is attached in both
// cases: when the search failed, it shows how many riders the search would
// otherwise have been choosing from.
//
// With an active target trace, the target rider's per-filter decision is
// logged as well.
func (s *DeliveryService) traceEligibility(ctx context.Context, trigger string, orderID, deliveryOrderID int, pickupLat, pickupLng float64, riders []models.NearbyRider, searchErr error) {
	fields := dispatchtrace.Fields{
		"order_id":          orderID,
		"delivery_order_id": deliveryOrderID,
		"trigger":           trigger,
		"radius_km":         s.searchRadiusKm,
		"eligible_count":    len(riders),
	}
	event := dispatchtrace.EventEligibilityEvaluated
	switch {
	case searchErr != nil:
		fields["result"] = "error"
		fields["reason_code"] = dispatchtrace.ReasonEligibilityQueryFailed
		fields["error"] = dispatchtrace.ErrorText(searchErr)
	case len(riders) == 0:
		event = dispatchtrace.EventNoEligibleRiders
		fields["result"] = "none"
		fields["reason_code"] = dispatchtrace.ReasonNoEligibleRiders
	default:
		fields["result"] = "ok"
	}

	if searchErr != nil || len(riders) == 0 {
		summary, err := s.deliveryRepo.GetRiderEligibilitySummary(ctx, pickupLat, pickupLng, s.searchRadiusKm)
		if err != nil {
			fields["funnel_error"] = dispatchtrace.ErrorText(err)
		} else {
			fields["online"] = summary.OnlineRiders
			// Riders removed by each filter, in the order the search applies them.
			fields["rejected_counts"] = map[string]int{
				"rider_account_or_state_ineligible": summary.OnlineRiders - summary.AccountEligibleRiders,
				"rider_not_available":               summary.AccountEligibleRiders - summary.AvailableRiders,
				"rider_location_missing":            summary.AvailableRiders - summary.RidersWithLocation,
				"rider_location_invalid":            summary.RidersWithLocation - summary.RidersWithValidLocation,
				"rider_location_stale_or_future":    summary.RidersWithValidLocation - summary.RidersWithFreshGPS,
				"rider_outside_radius":              summary.RidersWithFreshGPS - summary.RidersWithinRadius,
			}
			fields["would_be_eligible"] = summary.RidersWithinRadius
		}
	}
	dispatchtrace.Emit(event, fields)

	if !s.trace.Covers(orderID, time.Now()) {
		return
	}
	decision, err := s.deliveryRepo.RiderDecisionVector(ctx, s.trace.RiderID, pickupLat, pickupLng, s.searchRadiusKm)
	vector := dispatchtrace.Fields{
		"order_id": orderID,
		"trigger":  trigger,
		"rider_id": s.trace.RiderID,
	}
	if err != nil {
		vector["error"] = dispatchtrace.ErrorText(err)
	} else {
		for k, v := range decision.Fields() {
			vector[k] = v
		}
		offered := false
		for _, r := range riders {
			if r.RiderID == s.trace.RiderID {
				offered = true
			}
		}
		vector["selected_by_search"] = offered
	}
	dispatchtrace.Emit(dispatchtrace.EventTargetRiderDecision, vector)
}

func BuildDeliveryOrderRequestPayload(req *models.DeliveryOrderRequest, order *models.DeliveryOrder, distanceKm float64, expiresAt time.Time) map[string]interface{} {
	return map[string]interface{}{
		"request_id":           req.RequestID,
		"order_id":             order.OrderID,
		"restaurant_id":        order.RestaurantID,
		"restaurant_name":      order.RestaurantName,
		"restaurant_phone":     order.RestaurantPhone,
		"pickup_address":       order.PickupAddress,
		"drop_address":         "Delivery details available after acceptance",
		"pickup_latitude":      order.PickupLatitude,
		"pickup_longitude":     order.PickupLongitude,
		"delivery_distance_km": approximateDeliveryDistance(order),
		"distance_km":          distanceKm,
		"amount":               order.Amount,
		"payment_mode":         order.PaymentMode,
		"expires_at":           expiresAt.Format(time.RFC3339),
		"assignment_type":      order.AssignmentType,
		// Phase 6: "food" or "grocery". The rider app badges the card with it
		// and sends it back on every call about this delivery. Older builds
		// ignore the field and keep working, because every delivery they can
		// see is a food one.
		"order_type": models.NormalizeSourceOrderType(order.OrderType),
		// For a grocery delivery the pickup is a shop; the restaurant_* keys
		// carry it too, so one app code path reads either.
		"merchant_name": order.RestaurantName,
		"items_summary": order.ItemsSummary,
	}
}

// ProcessRiderAssignedEvent handles a RIDER_ASSIGNED_TO_ORDER event from restaurant-service.
func (s *DeliveryService) ProcessRiderAssignedEvent(ctx context.Context, evt *models.RiderAssignedToOrderEvent) error {
	allowed, err := s.deliveryRepo.OwnAssignmentEventCurrent(ctx, evt.OrderID, evt.RiderUserID)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
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
	_ = s.deliveryRepo.MarkEventProcessed(ctx, eventID, evt.OrderID, "RIDER_ASSIGNED_TO_ORDER", models.SourceOrderTypeFood)

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
func (s *DeliveryService) GetRiderOrderDetail(ctx context.Context, orderID int, riderUserID, orderType string) (*models.DeliveryOrder, error) {
	do, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID, orderType)
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
	s.IndexRiderLocation(ctx, riderID, lat, lng)

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
// IndexRiderLocation copies a location that PostgreSQL has already stored
// into the Redis dispatch index. Both location routes call it after their
// PostgreSQL write succeeds:
//
//	POST /api/v1/location/update  (rider app)  → LocationService.UpdateLocation
//	POST /api/v1/riders/location               → DeliveryService.UpdateRiderLocation
//
// Only /riders/location used to reach Redis, and the app never calls it, so
// the index stayed empty. Best effort: a Redis failure is logged (without
// coordinates) and never fails the upload; dispatch falls back to SQL.
func (s *DeliveryService) IndexRiderLocation(ctx context.Context, riderID string, lat, lng float64) {
	if s.dispatchCache == nil || !s.dispatchCache.Enabled() {
		return
	}
	if err := s.dispatchCache.UpdateRiderLocation(ctx, riderID, lat, lng); err != nil {
		dispatchtrace.Emit(dispatchtrace.EventLocationIndexFailed, dispatchtrace.Fields{
			"rider_id": riderID,
			"error":    dispatchtrace.ErrorText(err),
		})
	}
}

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
		if nonEmptyString(order.AssignedRiderID) || order.DeliveryStatus == models.DeliveryStatusCancelled || order.DeliveryStatus == models.DeliveryStatusDelivered {
			continue
		}
		if !order.IsGrocery() {
			allowed, err := s.deliveryRepo.FoodDispatchAllowed(ctx, order.OrderID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
		}
		eligible, err := s.deliveryRepo.FindNearestRidersAmong(ctx, order.PickupLatitude, order.PickupLongitude, s.searchRadiusKm, 1, []string{riderID})
		if err != nil {
			return nil, err
		}
		if len(eligible) == 0 {
			continue
		}
		payloads = append(payloads, BuildDeliveryOrderRequestPayload(req, order, req.DistanceKm, req.ExpiresAt))
	}
	return payloads, nil
}

// AcceptRequest handles POST /riders/order-requests/{requestId}/accept with row locking.
func (s *DeliveryService) AcceptRequest(ctx context.Context, requestID int, riderID string) (*models.DeliveryOrder, error) {
	order, err := s.acceptRequest(ctx, requestID, riderID)
	if err != nil {
		dispatchtrace.Emit(dispatchtrace.EventOfferAcceptRejected, dispatchtrace.Fields{
			"request_id":  requestID,
			"rider_id":    riderID,
			"reason_code": acceptRejectReason(err),
		})
		return nil, err
	}
	dispatchtrace.Emit(dispatchtrace.EventOfferAccepted, dispatchtrace.Fields{
		"order_id":          order.OrderID,
		"delivery_order_id": order.DeliveryOrderID,
		"request_id":        requestID,
		"rider_id":          riderID,
	})
	return order, nil
}

// acceptRejectReason maps acceptRequest's fixed error messages to reason codes.
func acceptRejectReason(err error) string {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "request not found"):
		return dispatchtrace.ReasonOfferNotFound
	case strings.HasPrefix(msg, "request does not belong"):
		return dispatchtrace.ReasonNotOwner
	case strings.HasPrefix(msg, "request already responded"):
		return dispatchtrace.ReasonNotPending
	case strings.HasPrefix(msg, "request has expired"):
		return dispatchtrace.ReasonOfferExpired
	case strings.Contains(msg, "already assigned"):
		return dispatchtrace.ReasonAlreadyAssigned
	default:
		return dispatchtrace.ReasonAcceptFailed
	}
}

// acceptRequest holds the accept transaction. Exclusivity comes from locking
// the request row and from AssignRider's `assigned_rider_id IS NULL` guard.
func (s *DeliveryService) acceptRequest(ctx context.Context, requestID int, riderID string) (*models.DeliveryOrder, error) {
	initial, err := s.deliveryRepo.GetRequestByID(ctx, requestID)
	if err != nil || initial.RiderID != riderID {
		return nil, fmt.Errorf("request not found")
	}
	initialOrder, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, initial.DeliveryOrderID)
	if err != nil {
		return nil, err
	}
	tx, err := s.deliveryRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if !initialOrder.IsGrocery() {
		if err := s.deliveryRepo.LockFoodOrder(ctx, tx, initial.OrderID); err != nil {
			return nil, err
		}
	}

	// Lock order: delivery order, then offer. Concurrent accepts for one
	// order queue here; the one that waits then sees the order assigned.
	if _, err := s.deliveryRepo.LockDeliveryOrderForRequest(ctx, tx, requestID); err != nil {
		return nil, fmt.Errorf("request not found")
	}

	// Lock the request row
	req, err := s.deliveryRepo.GetRequestByIDForUpdate(ctx, tx, requestID)
	if err != nil {
		return nil, fmt.Errorf("request not found")
	}
	if req.RiderID != riderID {
		return nil, fmt.Errorf("request does not belong to this rider")
	}
	if req.Status == models.RequestStatusAccepted {
		current, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, req.DeliveryOrderID)
		if err == nil && current.AssignedRiderID != nil && *current.AssignedRiderID == riderID && current.DeliveryStatus != models.DeliveryStatusCancelled {
			if !current.IsGrocery() {
				snap, err := s.deliveryRepo.GetOrderRiderSnapshot(ctx, current.OrderID)
				if err != nil {
					return nil, err
				}
				if !snap.HasRider(riderID) || restaurantOrderClosed(snap.OrderStatus) {
					return nil, fmt.Errorf("order is no longer active or assigned to this rider")
				}
			}
			return current, nil
		}
		return nil, fmt.Errorf("order is no longer assigned to this rider")
	}
	if req.Status == models.RequestStatusCancelled {
		// Offers are cancelled only by CancelOtherRequests, i.e. because
		// another rider accepted this order first.
		return nil, fmt.Errorf("order already assigned to another rider")
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
	if !deliveryOrder.IsGrocery() {
		if err := s.deliveryRepo.ClaimFoodOrder(ctx, tx, req.OrderID, riderID, s.searchRadiusKm); err != nil {
			return nil, err
		}
	}

	// Accept the request
	if err := s.deliveryRepo.AcceptRequest(ctx, tx, requestID); err != nil {
		return nil, fmt.Errorf("failed to accept: %w", err)
	}

	// Assign rider to delivery order.
	//
	// This UPDATE is guarded by `assigned_rider_id IS NULL`, so it — not the
	// Redis lock above — is what actually makes concurrent accepts safe: two
	// riders racing on the same delivery order serialise on the row and the
	// loser matches zero rows. The Redis lock only short-circuits earlier when
	// it is configured, and it deliberately falls through on error.
	if err := s.deliveryRepo.AssignRider(ctx, tx, req.DeliveryOrderID, riderID); err != nil {
		// The loser of the race gets the same clear, rider-facing message as
		// the pre-checks above rather than a wrapped internal error.
		if strings.Contains(err.Error(), "already assigned") {
			log.Printf("[DELIVERY] Late accept rejected order_id=%d rider_id=%s", req.OrderID, riderID)
			return nil, fmt.Errorf("order already assigned to another rider")
		}
		return nil, fmt.Errorf("failed to assign rider: %w", err)
	}

	// Cancel other pending requests for same order
	cancelledRiderIDs, err := s.deliveryRepo.CancelOtherRequests(ctx, tx, req.DeliveryOrderID, requestID)
	if err != nil {
		// The transaction is aborted after this; carrying on only produced a
		// misleading "failed to update rider availability" further down.
		log.Printf("[DELIVERY] Failed to cancel other requests: %v", err)
		return nil, fmt.Errorf("failed to withdraw other offers: %w", err)
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
	s.notifyOtherRiders(ctx, req.DeliveryOrderID, riderID, req.OrderID, cancelledRiderIDs)

	// Notify customer
	s.hub.SendToOrder(strconv.Itoa(req.OrderID), ws.WSMessage{
		Type: "RIDER_ASSIGNED",
		Data: map[string]interface{}{
			"order_id": req.OrderID,
			"rider_id": riderID,
		},
	})

	// Record the rider on restaurant-service's order, which is what the
	// customer app reads. If this never lands, the next status update retries
	// it synchronously (prepareRestaurantTransition).
	s.syncRiderAssignmentAsync(req.OrderID, riderID, now, deliveryOrder.OrderType)

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
	if req.Status == models.RequestStatusRejected {
		return nil // A retry after a lost decline response is already complete.
	}
	if req.Status != models.RequestStatusPending {
		return fmt.Errorf("request already responded to")
	}

	if err := s.deliveryRepo.RejectRequest(ctx, requestID); err != nil {
		return err
	}
	log.Printf("[DELIVERY] Rider %s rejected request %d for order %d", riderID, requestID, req.OrderID)
	dispatchtrace.Emit(dispatchtrace.EventOfferDeclined, dispatchtrace.Fields{
		"order_id":   req.OrderID,
		"request_id": requestID,
		"rider_id":   riderID,
	})

	// Check if all requests are now rejected/expired
	s.checkAllRequestsDone(ctx, req.DeliveryOrderID)
	return nil
}

// UpdateDeliveryStatus handles POST /riders/orders/{orderId}/status
func (s *DeliveryService) UpdateDeliveryStatus(ctx context.Context, orderID int, riderID string, newStatus string, paymentCollected bool, notes, orderType string) error {
	deliveryOrder, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID, orderType)
	if err != nil {
		return rejectStatus(orderID, riderID, "", newStatus,
			ErrCodeDeliveryNotFound, dispatchtrace.ReasonDeliveryNotFound, "delivery order not found", nil)
	}
	oldStatus := deliveryOrder.DeliveryStatus

	// Validate rider ownership: check both assigned_rider_id and rider_user_id
	owns := false
	if deliveryOrder.AssignedRiderID != nil && *deliveryOrder.AssignedRiderID == riderID {
		owns = true
	}
	if deliveryOrder.RiderUserID != nil && *deliveryOrder.RiderUserID == riderID {
		owns = true
	}
	if !owns {
		return rejectStatus(orderID, riderID, oldStatus, newStatus,
			ErrCodeNotAssignedRider, dispatchtrace.ReasonNotAssignedRider, "order not assigned to this rider", nil)
	}

	// A retry of the step that already committed (a tap whose response was
	// lost) succeeds without repeating side effects.
	if newStatus == oldStatus {
		dispatchtrace.Emit(dispatchtrace.EventDeliveryStatusUpdated, dispatchtrace.Fields{
			"order_id":    orderID,
			"rider_id":    riderID,
			"from_status": oldStatus,
			"to_status":   newStatus,
			"result":      "unchanged",
			"reason_code": dispatchtrace.ReasonAlreadyInStatus,
		})
		return nil
	}

	// The owner may end an order mid-delivery (complete, cancel, reject).
	// Nothing the rider does can succeed then; release the rider instead.
	snap := s.orderSnapshot(ctx, orderID)
	if snap != nil && restaurantOrderClosed(snap.OrderStatus) {
		s.releaseClosedDelivery(ctx, repository.ClosedDelivery{
			DeliveryOrderID: deliveryOrder.DeliveryOrderID,
			OrderID:         orderID,
			RiderID:         riderID,
			OrderStatus:     snap.OrderStatus,
		})
		return rejectStatus(orderID, riderID, oldStatus, newStatus,
			ErrCodeOrderClosed, dispatchtrace.ReasonRestaurantClosedOrder, "order was closed by the restaurant",
			dispatchtrace.Fields{"restaurant_status": snap.OrderStatus})
	}

	if !models.IsValidDeliveryTransition(oldStatus, newStatus) &&
		!isRestaurantOwnedDeliveryTransition(deliveryOrder, newStatus) {
		return rejectStatus(orderID, riderID, oldStatus, newStatus,
			ErrCodeInvalidTransition, dispatchtrace.ReasonInvalidTransition,
			fmt.Sprintf("invalid transition from '%s' to '%s'", oldStatus, newStatus),
			dispatchtrace.Fields{"expected_status": nextDeliveryStatus(deliveryOrder)})
	}

	// Validate COD payment collection when marking as delivered
	if newStatus == models.DeliveryStatusDelivered {
		isCOD := deliveryOrder.PaymentMode == "cod" || deliveryOrder.PaymentMode == "cash"
		if isCOD && !paymentCollected {
			return rejectStatus(orderID, riderID, oldStatus, newStatus,
				ErrCodeCashNotConfirmed, dispatchtrace.ReasonCashNotConfirmed, "cash collection confirmation required", nil)
		}
	}

	// The restaurant order owns the canonical lifecycle. Commit that transition
	// first; only then advance this service's delivery projection.
	if requiresRestaurantTransition(newStatus) {
		if s.restaurantCli == nil {
			return restaurantTransitionError(deliveryOrder, riderID, newStatus, fmt.Errorf("restaurant-service client not configured"))
		}
		if err := s.prepareRestaurantTransition(ctx, deliveryOrder, riderID, newStatus, snap); err != nil {
			return err
		}
		if deliveryOrder.IsGrocery() {
			if err := s.restaurantCli.NotifyGroceryDeliveryStatus(orderID, client.GroceryDeliveryStatusPayload{
				RiderID:        riderID,
				DeliveryStatus: newStatus,
				Reason:         notes,
			}); err != nil {
				return restaurantTransitionError(deliveryOrder, riderID, newStatus, err)
			}
		} else if err := s.restaurantCli.NotifyDeliveryStatusUpdate(orderID, client.DeliveryStatusPayload{
			OrderID:          orderID,
			RestaurantID:     deliveryOrder.RestaurantID,
			RiderID:          riderID,
			DeliveryStatus:   newStatus,
			PaymentCollected: paymentCollected,
			Notes:            notes,
		}); err != nil {
			return restaurantTransitionError(deliveryOrder, riderID, newStatus, err)
		}
	} else {
		s.ensureAssignmentRecorded(ctx, deliveryOrder, riderID, snap)
	}

	changed, err := s.deliveryRepo.AdvanceDelivery(ctx, deliveryOrder, riderID, newStatus)
	if err != nil {
		return rejectStatus(orderID, riderID, oldStatus, newStatus, ErrCodeStatusWriteFailed,
			dispatchtrace.ReasonStatusWriteFailed, "failed to save delivery status; refresh and retry", nil)
	}
	if !changed {
		return nil
	}
	dispatchtrace.Emit(dispatchtrace.EventDeliveryStatusUpdated, dispatchtrace.Fields{
		"order_id": orderID, "rider_id": riderID, "from_status": oldStatus, "to_status": newStatus, "result": "ok",
	})
	if newStatus == models.DeliveryStatusDelivered {
		if availability, err := s.deliveryRepo.GetRiderAvailability(ctx, riderID); err == nil {
			_ = s.riderRepo.SetAvailability(ctx, riderID, availability.IsOnline)
		}
		_ = s.riderRepo.SetOnTrip(ctx, riderID, false)
		s.notifyRiderReferral(ctx, riderID, orderID)
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
func (s *DeliveryService) GetDeliveryTracking(ctx context.Context, orderID int, orderType string) (*models.DeliveryTrackingResponse, error) {
	do, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID, orderType)
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

func (s *DeliveryService) notifyOtherRiders(ctx context.Context, deliveryOrderID int, acceptedRiderID string, orderID int, cancelledRiderIDs []string) {
	// Send WS event to riders whose requests were cancelled
	for _, rID := range cancelledRiderIDs {
		if rID != acceptedRiderID {
			dispatchtrace.Emit(dispatchtrace.EventOfferWithdrawn, dispatchtrace.Fields{
				"order_id":    orderID,
				"rider_id":    rID,
				"reason_code": dispatchtrace.ReasonAssignedToOther,
			})
			s.hub.SendToRider(rID, ws.WSMessage{
				Type: "ORDER_ASSIGNED_TO_OTHER_RIDER",
				Data: map[string]interface{}{
					"order_id": orderID,
				},
			})
		}
	}
	log.Printf("[DELIVERY] Notified %d other riders about order %d assignment", len(cancelledRiderIDs), orderID)
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

			var orderID int
			deliveryOrder, err := s.deliveryRepo.GetDeliveryOrderByID(ctx, deliveryOrderID)
			if err == nil && deliveryOrder != nil {
				orderID = deliveryOrder.OrderID
				if s.dispatchCache != nil && s.dispatchCache.Enabled() {
					s.dispatchCache.ClearPendingOrder(ctx, deliveryOrder.OrderID)
				}
				s.hub.SendToOrder(strconv.Itoa(orderID), ws.WSMessage{
					Type: "DELIVERY_STATUS_UPDATED",
					Data: map[string]interface{}{
						"order_id":        orderID,
						"delivery_status": models.DeliveryStatusNoRiderFound,
					},
				})
			}
			log.Printf("[DELIVERY] All requests rejected/expired for delivery_order %d, marked no_rider_found", deliveryOrderID)
		}
	}
}

// notifyRiderReferral tells restaurant-service that a rider completed a
// delivery, so the referral domain can decide whether it qualifies a referral.
//
// Everything here is best-effort and non-blocking. The delivery has already
// been committed by the time this runs; a referral is worth strictly less than
// a delivery, so no failure in this path may affect the rider's order.
//
// Restaurant-owned deliveries are included deliberately: the rider genuinely
// completed a delivery, which is what the referral rule is about, even though
// the platform pays no delivery fee for it.
func (s *DeliveryService) notifyRiderReferral(ctx context.Context, riderID string, orderID int) {
	if !s.riderReferralEnabled || s.restaurantCli == nil || riderID == "" {
		return
	}

	completed, err := s.deliveryRepo.CountCompletedDeliveries(ctx, riderID)
	if err != nil {
		// Without a trustworthy count the referral rule cannot be evaluated
		// correctly, and guessing could pay out early. Skip this delivery and
		// let the next one carry the correct count.
		log.Printf("[DELIVERY] Referral callback skipped for order %d: delivery count failed: %v", orderID, err)
		return
	}

	s.restaurantCli.NotifyRiderDeliveryCompletedAsync(client.RiderDeliveryCompletedPayload{
		RiderID: riderID,
		// Scoped by order so a retry of the same delivery dedupes in the
		// referral outbox instead of qualifying a referral twice.
		DeliveryRef:         fmt.Sprintf("order:%d", orderID),
		CompletedDeliveries: completed,
	})
}

// requiresOwnRiderCheck reports whether an ORDER_PLACED event must be
// re-checked for a live own rider before platform dispatch.
//
// Only an explicitly restaurant-owned event is. Everything else — including an
// empty mode — dispatches: restaurant-service has already decided, and an empty
// mode has always dispatched in practice (see the comment at the call site).
func requiresOwnRiderCheck(deliveryMode string) bool {
	switch strings.ToLower(strings.TrimSpace(deliveryMode)) {
	case "restaurant_own_rider", "restaurant_owned":
		return true
	default:
		return false
	}
}

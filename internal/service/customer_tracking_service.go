package service

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/maps"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

const customerTrackingStaleAfter = 90 * time.Second

type CustomerTrackingRouteService interface {
	GetDeliveryRoute(context.Context, string, int, string, maps.Coordinate) (*DeliveryRoute, error)
}

func (s *DeliveryService) GetCustomerTrackingSnapshot(ctx context.Context, orderID int, orderType string, routeSvc CustomerTrackingRouteService) (*models.CustomerTrackingSnapshot, error) {
	do, err := s.deliveryRepo.GetDeliveryOrderByOrderID(ctx, orderID, orderType)
	if err != nil {
		return nil, err
	}
	resp := &models.CustomerTrackingSnapshot{
		OrderID:        do.OrderID,
		OrderType:      models.NormalizeSourceOrderType(do.OrderType),
		DeliveryStatus: do.DeliveryStatus,
		Phase:          customerTrackingPhase(do.DeliveryStatus),
		Timeline:       buildTimeline(do),
	}
	riderID := customerTrackingAssignedRiderID(do)
	if riderID == "" || !customerTrackingAllowsPreciseLocation(do.DeliveryStatus) {
		return resp, nil
	}
	loc, err := s.deliveryRepo.GetRiderLocation(ctx, riderID)
	if err != nil {
		if err != sql.ErrNoRows {
			dispatchtrace.Emit("customer_tracking_internal_lookup", dispatchtrace.Fields{
				"order_id": orderID,
				"success":  false,
				"error":    dispatchtrace.ErrorText(err),
			})
		}
		return resp, nil
	}
	stale := time.Since(loc.LastUpdatedAt.UTC()) > customerTrackingStaleAfter
	resp.Rider = &models.CustomerTrackingRider{
		Latitude:          loc.Latitude,
		Longitude:         loc.Longitude,
		LocationUpdatedAt: loc.LastUpdatedAt.UTC().Format(time.RFC3339Nano),
		Stale:             stale,
	}
	if routeSvc == nil || stale {
		return resp, nil
	}
	route, err := routeSvc.GetDeliveryRoute(ctx, riderID, do.OrderID, do.OrderType, maps.Coordinate{
		Latitude:  loc.Latitude,
		Longitude: loc.Longitude,
	})
	if err != nil {
		dispatchtrace.Emit("customer_tracking_route_error", dispatchtrace.Fields{
			"order_id": orderID,
			"success":  false,
			"error":    dispatchtrace.ErrorText(err),
		})
		return resp, nil
	}
	resp.Route = customerTrackingRouteDTO(route)
	return resp, nil
}

func customerTrackingAssignedRiderID(do *models.DeliveryOrder) string {
	if do == nil {
		return ""
	}
	if do.AssignedRiderID != nil && strings.TrimSpace(*do.AssignedRiderID) != "" {
		return strings.TrimSpace(*do.AssignedRiderID)
	}
	if do.RiderUserID != nil {
		return strings.TrimSpace(*do.RiderUserID)
	}
	return ""
}

func customerTrackingAllowsPreciseLocation(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case models.DeliveryStatusRiderAssigned,
		models.DeliveryStatusRiderArrivedRestaurant,
		models.DeliveryStatusPickedUp,
		models.DeliveryStatusOnTheWay:
		return true
	default:
		return false
	}
}

func customerTrackingPhase(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case models.DeliveryStatusPickedUp, models.DeliveryStatusOnTheWay:
		return "delivery"
	case models.DeliveryStatusRiderAssigned, models.DeliveryStatusRiderArrivedRestaurant:
		return "rider_to_pickup"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func customerTrackingRouteDTO(route *DeliveryRoute) *models.CustomerTrackingRoute {
	if route == nil {
		return nil
	}
	etaMinutes := 0
	if route.DurationSeconds > 0 {
		etaMinutes = int(math.Ceil(route.DurationSeconds / 60))
	}
	now := time.Now().UTC()
	return &models.CustomerTrackingRoute{
		DestinationType:       route.DestinationType,
		EncodedPolyline:       route.EncodedPolyline,
		DistanceMeters:        route.DistanceMeters,
		DurationSeconds:       route.DurationSeconds,
		StaticDurationSeconds: route.StaticDurationSeconds,
		TrafficDelaySeconds:   route.TrafficDelaySeconds,
		ETASeconds:            route.DurationSeconds,
		ETAMinutes:            etaMinutes,
		GeneratedAt:           route.GeneratedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:             route.ExpiresAt.UTC().Format(time.RFC3339Nano),
		Stale:                 !route.ExpiresAt.IsZero() && now.After(route.ExpiresAt.UTC()),
		FallbackReason:        route.FallbackReason,
	}
}

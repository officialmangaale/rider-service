package models

import "time"

// DeliveryOrder represents a rider-service-owned delivery order
// created from ORDER_PLACED SQS events.
type DeliveryOrder struct {
	DeliveryOrderID int        `json:"delivery_order_id"`
	OrderID         int        `json:"order_id"`
	RestaurantID    int        `json:"restaurant_id"`
	CustomerID      int        `json:"customer_id"`
	PickupLatitude  float64    `json:"pickup_latitude"`
	PickupLongitude float64    `json:"pickup_longitude"`
	PickupAddress   string     `json:"pickup_address"`
	DropLatitude    float64    `json:"drop_latitude"`
	DropLongitude   float64    `json:"drop_longitude"`
	DropAddress     string     `json:"drop_address"`
	Amount          float64    `json:"amount"`
	PaymentMode     string     `json:"payment_mode"`
	DeliveryStatus  string     `json:"delivery_status"`
	AssignedRiderID *string    `json:"assigned_rider_id"`
	RiderUserID     *string    `json:"rider_user_id"`
	AssignmentType  string     `json:"assignment_type"`
	RestaurantOwned bool       `json:"restaurant_owned"`
	RestaurantName  string     `json:"restaurant_name"`
	RestaurantPhone string     `json:"restaurant_phone"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	AssignedAt      *time.Time `json:"assigned_at"`
	PickedUpAt      *time.Time `json:"picked_up_at"`
	DeliveredAt     *time.Time `json:"delivered_at"`
}

// DeliveryOrderRequest represents a pending request sent to a nearby rider.
type DeliveryOrderRequest struct {
	RequestID       int       `json:"request_id"`
	DeliveryOrderID int       `json:"delivery_order_id"`
	OrderID         int       `json:"order_id"`
	RiderID         string    `json:"rider_id"`
	Status          string    `json:"status"`
	DistanceKm      float64   `json:"distance_km"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Request statuses
const (
	RequestStatusPending   = "pending"
	RequestStatusAccepted  = "accepted"
	RequestStatusRejected  = "rejected"
	RequestStatusExpired   = "expired"
	RequestStatusCancelled = "cancelled"
)

// Delivery statuses
const (
	DeliveryStatusPending              = "pending"
	DeliveryStatusRiderSearching       = "rider_searching"
	DeliveryStatusRiderAssigned        = "rider_assigned"
	DeliveryStatusRiderArrivedRestaurant = "rider_arrived_restaurant"
	DeliveryStatusPickedUp             = "picked_up"
	DeliveryStatusOnTheWay             = "on_the_way"
	DeliveryStatusDelivered            = "delivered"
	DeliveryStatusNoRiderFound         = "no_rider_found"
	DeliveryStatusCancelled            = "cancelled"
)

// Allowed delivery status transitions for rider
var DeliveryStatusTransitions = map[string][]string{
	DeliveryStatusRiderAssigned:          {DeliveryStatusRiderArrivedRestaurant},
	DeliveryStatusRiderArrivedRestaurant: {DeliveryStatusPickedUp},
	DeliveryStatusPickedUp:               {DeliveryStatusOnTheWay},
	DeliveryStatusOnTheWay:               {DeliveryStatusDelivered},
}

// IsValidDeliveryTransition checks if transition is allowed.
func IsValidDeliveryTransition(from, to string) bool {
	allowed, exists := DeliveryStatusTransitions[from]
	if !exists {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// ProcessedEvent for SQS event idempotency.
type ProcessedEvent struct {
	EventID     string    `json:"event_id"`
	OrderID     *int      `json:"order_id"`
	EventType   string    `json:"event_type"`
	ProcessedAt time.Time `json:"processed_at"`
}

// RiderLocation represents current rider GPS position.
type RiderLocation struct {
	RiderID       string    `json:"rider_id"`
	Latitude      float64   `json:"latitude"`
	Longitude     float64   `json:"longitude"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// RiderAvailability represents rider's online/available status.
type RiderAvailability struct {
	RiderID        string `json:"rider_id"`
	IsOnline       bool   `json:"is_online"`
	IsAvailable    bool   `json:"is_available"`
	CurrentOrderID *int   `json:"current_order_id"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// NearbyRider used in nearest rider search results.
type NearbyRider struct {
	RiderID    string  `json:"rider_id"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	DistanceKm float64 `json:"distance_km"`
}

// OrderPlacedEvent is the SQS message payload from restaurant-service.
type OrderPlacedEvent struct {
	EventType  string         `json:"event_type"`
	EventID    string         `json:"event_id"`
	OrderID    int            `json:"order_id"`
	RestaurantID int          `json:"restaurant_id"`
	CustomerID int            `json:"customer_id"`
	OrderType  string         `json:"order_type"`
	PaymentMode string        `json:"payment_mode"`
	Amount     float64        `json:"amount"`
	Pickup     LocationDetail `json:"pickup"`
	Drop       LocationDetail `json:"drop"`
	CreatedAt  string         `json:"created_at"`
}

// LocationDetail for pickup/drop coordinates.
type LocationDetail struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Address   string  `json:"address"`
}

// RiderAssignedToOrderEvent is the SQS message payload when a restaurant assigns its own rider.
type RiderAssignedToOrderEvent struct {
	EventType    string `json:"event_type"`
	OrderID      int    `json:"order_id"`
	RestaurantID int    `json:"restaurant_id"`
	RiderUserID  string `json:"rider_user_id"`
	RiderName    string `json:"rider_name"`
	RiderPhone   string `json:"rider_phone"`
	AssignedAt   string `json:"assigned_at"`
}

// DeliveryTrackingResponse for GET /delivery/orders/{orderId}/tracking
type DeliveryTrackingResponse struct {
	OrderID        int                    `json:"order_id"`
	DeliveryStatus string                 `json:"delivery_status"`
	Pickup         LocationDetail         `json:"pickup"`
	Drop           LocationDetail         `json:"drop"`
	Rider          *TrackingRiderInfo     `json:"rider,omitempty"`
	Timeline       []DeliveryTimelineItem `json:"timeline"`
}

// TrackingRiderInfo for customer tracking.
type TrackingRiderInfo struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Phone     string  `json:"phone"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// DeliveryTimelineItem for tracking timeline.
type DeliveryTimelineItem struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

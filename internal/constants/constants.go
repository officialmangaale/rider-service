package constants

// OrderStatus matches the real order_status_enum in PostgreSQL.
type OrderStatus string

const (
	OrderPending        OrderStatus = "pending"
	OrderPlaced         OrderStatus = "placed"
	OrderAccepted       OrderStatus = "accepted"
	OrderPreparing      OrderStatus = "preparing"
	OrderReady          OrderStatus = "ready"
	OrderOutForDelivery OrderStatus = "out_for_delivery"
	OrderDelivered      OrderStatus = "delivered"
	OrderCancelled      OrderStatus = "cancelled"
)

// RiderAllowedTransitions defines which order status transitions a rider can perform.
// Restaurant owns: pending→placed→accepted→preparing→ready.
// Rider owns: ready→out_for_delivery→delivered (and cancellation from certain states).
var RiderAllowedTransitions = map[OrderStatus][]OrderStatus{
	OrderReady:          {OrderOutForDelivery},         // picked up
	OrderOutForDelivery: {OrderDelivered, OrderCancelled}, // delivered or cancel
}

// IsRiderAllowedTransition checks if rider can move order between these states.
func IsRiderAllowedTransition(from, to OrderStatus) bool {
	allowed, exists := RiderAllowedTransitions[from]
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

// IsOrderActive returns true if the order is in an active delivery state for the rider.
func IsOrderActive(s OrderStatus) bool {
	return s == OrderReady || s == OrderOutForDelivery
}

// AssignmentStatus for delivery_assignments table.
type AssignmentStatus string

const (
	AssignmentPending  AssignmentStatus = "pending"
	AssignmentAccepted AssignmentStatus = "accepted"
	AssignmentRejected AssignmentStatus = "rejected"
	AssignmentExpired  AssignmentStatus = "expired"
)

// EarningType for rider_earnings table.
type EarningType string

const (
	EarningDeliveryFee EarningType = "delivery_fee"
	EarningTip         EarningType = "tip"
	EarningIncentive   EarningType = "incentive"
	EarningBonus       EarningType = "bonus"
	EarningPenalty     EarningType = "penalty"
)

// UserRole values matching users.primary_role.
const (
	RoleDeliveryDriver  = "delivery_driver"
	RoleCustomer        = "customer"
	RoleRestaurantOwner = "restaurant_owner"
	RoleAdmin           = "admin"
)

// RiderEvent represents rider-side tracking milestones.
// These do NOT change the shared order_status_enum — they are recorded
// in delivery_status_history as audit events for the Flutter app to consume.
type RiderEvent string

const (
	RiderEventArrivedAtRestaurant RiderEvent = "arrived_at_restaurant"
	RiderEventArrivedAtCustomer   RiderEvent = "arrived_at_customer"
)

package models

import (
	"database/sql"
	"time"
)

// User represents a row from the shared users table (rider-relevant fields).
type User struct {
	ID                        string          `json:"id"`         // UUID
	UserID                    int             `json:"user_id"`    // serial PK
	Phone                     *string         `json:"phone"`
	Email                     *string         `json:"email"`
	FirstName                 *string         `json:"first_name"`
	LastName                  *string         `json:"last_name"`
	DisplayName               *string         `json:"display_name"`
	AvatarURL                 *string         `json:"avatar_url"`
	PrimaryRole               *string         `json:"primary_role"`
	Status                    *string         `json:"status"`
	VehicleType               *string         `json:"vehicle_type"`
	VehicleRegistrationNumber *string         `json:"vehicle_registration_number"`
	VehicleDetails            *string         `json:"vehicle_details"`   // jsonb
	InsuranceDetails          *string         `json:"insurance_details"` // jsonb
	MaxCarryCapacityKg        *float64        `json:"max_carry_capacity_kg"`
	LicenseNumber             *string         `json:"license_number"`
	LicenseExpiry             *string         `json:"license_expiry"`
	IsAvailable               bool            `json:"is_available"`
	OnTrip                    bool            `json:"on_trip"`
	CurrentLat                *float64        `json:"current_lat"`
	CurrentLng                *float64        `json:"current_lng"`
	LastLocationUpdate        *time.Time      `json:"last_location_update"`
	KYCVerified               bool            `json:"kyc_verified"`
	KYCData                   *string         `json:"kyc_data"`         // jsonb
	VerificationDocs          *string         `json:"verification_docs"` // jsonb
	BankDetails               *string         `json:"bank_details"`      // jsonb
	PayoutMethods             *string         `json:"payout_methods"`    // jsonb
	RatingAvg                 *float64        `json:"rating_avg"`
	RatingCount               int             `json:"rating_count"`
	TotalDeliveries           int             `json:"total_deliveries"`
	TotalOrders               int             `json:"total_orders"`
	Earnings                  float64         `json:"earnings"`
	CreatedAt                 time.Time       `json:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at"`
}

// Order represents a row from the shared orders table (delivery-relevant fields).
type Order struct {
	OrderID             int            `json:"order_id"`
	CustomerID          *string        `json:"customer_id"`
	RestaurantID        int            `json:"restaurant_id"`
	DeliveryPartnerID   *string        `json:"delivery_partner_id"`
	OrderStatus         string         `json:"order_status"`
	PaymentStatus       *string        `json:"payment_status"`
	Subtotal            float64        `json:"subtotal"`
	TaxAmount           float64        `json:"tax_amount"`
	DeliveryFee         float64        `json:"delivery_fee"`
	TipAmount           float64        `json:"tip_amount"`
	DiscountAmount      float64        `json:"discount_amount"`
	TotalAmount         float64        `json:"total_amount"`
	DeliveryAddress     *string        `json:"delivery_address"`
	DeliveryLatitude    *float64       `json:"delivery_latitude"`
	DeliveryLongitude   *float64       `json:"delivery_longitude"`
	OrderType           *string        `json:"order_type"`
	OrderNumber         *string        `json:"order_number"`
	EstimatedDelivery   *time.Time     `json:"estimated_delivery_time"`
	ActualDelivery      *time.Time     `json:"actual_delivery_time"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	// Joined fields from restaurants table
	RestaurantName      string         `json:"restaurant_name,omitempty"`
	RestaurantAddress   string         `json:"restaurant_address,omitempty"`
	RestaurantLat       *float64       `json:"restaurant_lat,omitempty"`
	RestaurantLng       *float64       `json:"restaurant_lng,omitempty"`
}

// DeliveryAssignment represents a row from delivery_assignments.
type DeliveryAssignment struct {
	ID           string     `json:"id"`
	OrderID      int        `json:"order_id"`
	RiderID      string     `json:"rider_id"`
	Status       string     `json:"status"`
	OfferedAt    time.Time  `json:"offered_at"`
	RespondedAt  *time.Time `json:"responded_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RejectReason *string    `json:"reject_reason,omitempty"`
}

// RiderLocationHistory represents a row in rider_location_history.
type RiderLocationHistory struct {
	ID         string    `json:"id"`
	RiderID    string    `json:"rider_id"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Heading    *float64  `json:"heading"`
	Speed      *float64  `json:"speed"`
	RecordedAt time.Time `json:"recorded_at"`
}

// RiderEarning represents a row in rider_earnings.
type RiderEarning struct {
	ID          string    `json:"id"`
	RiderID     string    `json:"rider_id"`
	OrderID     *int      `json:"order_id"`
	Type        string    `json:"type"`
	Amount      float64   `json:"amount"`
	Description *string   `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// RiderNotification represents a row in rider_notifications.
type RiderNotification struct {
	ID        string    `json:"id"`
	RiderID   string    `json:"rider_id"`
	Title     string    `json:"title"`
	Body      *string   `json:"body"`
	Type      *string   `json:"type"`
	Data      *string   `json:"data"` // JSONB as string
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// DeliveryStatusHistory represents a row in delivery_status_history.
type DeliveryStatusHistory struct {
	ID         string    `json:"id"`
	OrderID    int       `json:"order_id"`
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	ChangedBy  *string   `json:"changed_by"`
	Metadata   *string   `json:"metadata"` // JSONB
	CreatedAt  time.Time `json:"created_at"`
}

// EarningsSummary is a computed view for dashboard.
type EarningsSummary struct {
	TodayEarnings float64 `json:"today_earnings"`
	WeekEarnings  float64 `json:"week_earnings"`
	MonthEarnings float64 `json:"month_earnings"`
	TotalOrders   int     `json:"total_orders"`
}

// DashboardData for the rider home screen.
type DashboardData struct {
	Rider       *User            `json:"rider"`
	ActiveOrder *Order           `json:"active_order"`
	Earnings    *EarningsSummary `json:"earnings"`
}

// OnboardingStatus tracks profile completion.
type OnboardingStatus struct {
	ProfileComplete  bool `json:"profile_complete"`
	VehicleComplete  bool `json:"vehicle_complete"`
	KYCComplete      bool `json:"kyc_complete"`
	BankComplete     bool `json:"bank_complete"`
	FullyOnboarded   bool `json:"fully_onboarded"`
}

// Ensure sql.NullString is available.
var _ = sql.NullString{}

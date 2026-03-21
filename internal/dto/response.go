package dto

import (
	"math"
	"time"

	"github.com/gin-gonic/gin"
)

// APIResponse matches the user-service response envelope:
// {"status":"success|error","statusCode":200,"message":"...","data":{...}}
type APIResponse struct {
	Status     string      `json:"status"`
	StatusCode int         `json:"statusCode"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// PaginatedData wraps paginated results.
type PaginatedData struct {
	Items      interface{} `json:"items"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination metadata matching restaurant-service pattern.
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Success sends a successful JSON response.
func Success(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, APIResponse{
		Status:     "success",
		StatusCode: statusCode,
		Message:    message,
		Data:       data,
	})
}

// ErrorResponse sends an error JSON response.
func ErrorResponse(c *gin.Context, statusCode int, message string, errDetail string) {
	c.JSON(statusCode, APIResponse{
		Status:     "error",
		StatusCode: statusCode,
		Message:    message,
		Error:      errDetail,
	})
}

// ValidationError sends a 400 validation error.
func ValidationError(c *gin.Context, message string) {
	ErrorResponse(c, 400, message, "Validation failed")
}

// Unauthorized sends a 401 response.
func Unauthorized(c *gin.Context, message string) {
	ErrorResponse(c, 401, message, "Unauthorized")
}

// Forbidden sends a 403 response.
func Forbidden(c *gin.Context, message string) {
	ErrorResponse(c, 403, message, "Forbidden")
}

// NotFound sends a 404 response.
func NotFound(c *gin.Context, message string) {
	ErrorResponse(c, 404, message, "Not found")
}

// Conflict sends a 409 response.
func Conflict(c *gin.Context, message string) {
	ErrorResponse(c, 409, message, "Conflict")
}

// InternalError sends a 500 response.
func InternalError(c *gin.Context, message string) {
	ErrorResponse(c, 500, message, "Internal server error")
}

// Paginated sends a paginated success response.
func Paginated(c *gin.Context, items interface{}, page, limit int, total int64) {
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	Success(c, 200, "fetched", PaginatedData{
		Items: items,
		Pagination: Pagination{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// OnboardingStatusResponse for GET /api/v1/rider/onboarding-status
type OnboardingStatusResponse struct {
	CurrentStatus   string   `json:"current_status"`
	CompletedSteps  []string `json:"completed_steps"`
	PendingSteps    []string `json:"pending_steps"`
	RejectionReason *string  `json:"rejection_reason"`
	IsReadyToRide   bool     `json:"is_ready_to_ride"`
}

// UpdateLocationResponse for POST /api/v1/location/update
type UpdateLocationResponse struct {
	IsAvailable        bool      `json:"is_available"`
	LastLocationUpdate time.Time `json:"last_location_update"`
}

// RiderAvailabilityResponse matches GET /api/v1/rider/availability
type RiderAvailabilityResponse struct {
	IsAvailable bool         `json:"is_is_available"` // The user JSON specified `is_available: true`
	ActiveShift *ActiveShift `json:"active_shift,omitempty"`
}

// ActiveShift keeps shift started at
type ActiveShift struct {
	StartedAt time.Time `json:"started_at"`
}

// LiveOrderResponse for GET /api/v1/orders/incoming
type LiveOrderResponse struct {
	Assignment *AssignmentResponse `json:"assignment,omitempty"`
	Order      *OrderResponse      `json:"order,omitempty"`
}

type AssignmentResponse struct {
	ID                 string    `json:"id"`
	DecisionDeadlineAt time.Time `json:"decision_deadline_at"`
}

type OrderResponse struct {
	ID               string  `json:"id"`
	AssignmentID     string  `json:"assignment_id,omitempty"`  // Used for active order
	RestaurantName   string  `json:"restaurant_name"`
	CustomerName     string  `json:"customer_name"`
	PickupAddress    string  `json:"pickup_address"`
	DeliveryAddress  string  `json:"delivery_address"`
	DistanceKm       float64 `json:"distance_km"`
	BasePayout       float64 `json:"base_payout"`
	DistancePayout   float64 `json:"distance_payout"`
	WaitingCharges   float64 `json:"waiting_charges"`
	SurgeBonus       float64 `json:"surge_bonus"`
	TipAmount        float64 `json:"tip_amount"`
	ItemsCount       int     `json:"items_count"`
	Status           string  `json:"status,omitempty"`         // Used for active order
	PaymentMethod    string  `json:"payment_method,omitempty"` // Used for active order
	CustomerPhone    string  `json:"customer_phone,omitempty"` // Used for active order
}

// RiderProfileResponse for GET /api/v1/rider/profile
type RiderProfileResponse struct {
	User    UserProfile    `json:"user"`
	Rider   RiderStats     `json:"rider"`
	Vehicle VehicleProfile `json:"vehicle"`
}

type UserProfile struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
	City      string `json:"city"`
}

type RiderStats struct {
	AvgRating          float64 `json:"avg_rating"`
	AvailabilityStatus string  `json:"availability_status"`
	ActiveHoursToday   float64 `json:"active_hours_today"`
}

type VehicleProfile struct {
	VehicleType    string `json:"vehicle_type"`
	RegistrationNo string `json:"registration_no"`
}

// EarningsSummaryResponse for GET /api/v1/earnings/summary
type EarningsSummaryResponse struct {
	TodayEarnings   float64 `json:"today_earnings"`
	WeeklyEarnings  float64 `json:"weekly_earnings"`
	MonthlyEarnings float64 `json:"monthly_earnings"`
	TotalEarnings   float64 `json:"total_earnings"`
	WalletBalance   float64 `json:"wallet_balance"`
	PendingPayout   float64 `json:"pending_payout"`
	SettledPayout   float64 `json:"settled_payout"`
}

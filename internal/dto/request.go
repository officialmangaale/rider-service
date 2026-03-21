package dto

// UpdateProfileRequest for rider profile fields in users table.
type UpdateProfileRequest struct {
	FirstName   *string `json:"first_name"`
	LastName    *string `json:"last_name"`
	Email       *string `json:"email"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// UpdateVehicleRequest for vehicle fields in users table.
type UpdateVehicleRequest struct {
	VehicleType               *string  `json:"vehicle_type"`
	VehicleRegistrationNumber *string  `json:"vehicle_registration_number"`
	VehicleDetails            *string  `json:"vehicle_details"`     // JSON string
	InsuranceDetails          *string  `json:"insurance_details"`   // JSON string
	MaxCarryCapacityKg        *float64 `json:"max_carry_capacity_kg"`
	LicenseNumber             *string  `json:"license_number"`
	LicenseExpiry             *string  `json:"license_expiry"` // date string
}

// UpdateBankDetailsRequest for bank/payout fields in users table.
type UpdateBankDetailsRequest struct {
	BankDetails  *string `json:"bank_details"`  // JSON string
	PayoutMethods *string `json:"payout_methods"` // JSON string
}

// UpdateKYCRequest for KYC fields in users table.
type UpdateKYCRequest struct {
	KYCData          *string `json:"kyc_data"`          // JSON string
	VerificationDocs *string `json:"verification_docs"` // JSON string
}

// UpdateLocationRequest for GPS location update.
type UpdateLocationRequest struct {
	Latitude  float64  `json:"latitude" binding:"required"`
	Longitude float64  `json:"longitude" binding:"required"`
	Heading   *float64 `json:"heading"`
	Speed     *float64 `json:"speed"`
}

// RejectAssignmentRequest for rejecting an order assignment.
type RejectAssignmentRequest struct {
	Reason string `json:"reason"`
}

// CancelDeliveryRequest for cancelling a delivery.
type CancelDeliveryRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// FailDeliveryRequest for marking delivery failed.
type FailDeliveryRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// DeviceTokenRequest for push notification registration.
type DeviceTokenRequest struct {
	Platform  string `json:"platform" binding:"required"`   // "android", "ios"
	PushToken string `json:"push_token" binding:"required"`
}

// PaginationQuery for paginated list endpoints.
type PaginationQuery struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}

// Normalize sets sane defaults and bounds for pagination.
func (pq *PaginationQuery) Normalize() {
	if pq.Page < 1 {
		pq.Page = 1
	}
	if pq.Limit < 1 {
		pq.Limit = 20
	}
	if pq.Limit > 100 {
		pq.Limit = 100
	}
}

// Offset calculates the SQL offset from page and limit.
func (pq *PaginationQuery) Offset() int {
	return (pq.Page - 1) * pq.Limit
}

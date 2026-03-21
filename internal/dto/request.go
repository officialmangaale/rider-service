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
	VehicleType          string `json:"vehicle_type"`
	Make                 string `json:"make"`
	Model                string `json:"model"`
	Year                 int    `json:"year"`
	RegistrationNumber   string `json:"registration_number"`
	RCDocumentURL        string `json:"rc_document_url"`
	InsuranceDocumentURL string `json:"insurance_document_url"`
}

// UpdateBankDetailsRequest for bank/payout fields in users table.
type UpdateBankDetailsRequest struct {
	AccountHolderName string `json:"account_holder_name"`
	AccountNumber     string `json:"account_number"`
	IFSCCode          string `json:"ifsc_code"`
	BankName          string `json:"bank_name"`
	BranchName        string `json:"branch_name"`
}

// UpdateKYCRequest for KYC fields in users table.
type UpdateKYCRequest struct {
	DrivingLicenseNumber   string `json:"driving_license_number"`
	DrivingLicenseFrontURL string `json:"driving_license_front_url"`
	DrivingLicenseBackURL  string `json:"driving_license_back_url"`
	NationalIDType         string `json:"national_id_type"`
	NationalIDNumber       string `json:"national_id_number"`
	NationalIDFrontURL     string `json:"national_id_front_url"`
	NationalIDBackURL      string `json:"national_id_back_url"`
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

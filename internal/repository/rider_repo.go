package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// RiderRepository provides data access for rider data in the shared users table.
type RiderRepository struct {
	db *sql.DB
}

// NewRiderRepository creates a new RiderRepository.
func NewRiderRepository(db *sql.DB) *RiderRepository {
	return &RiderRepository{db: db}
}

// riderSelectColumns — columns we SELECT for rider profiles from users table.
const riderSelectColumns = `
	u.id, u.user_id, u.phone, u.email, u.first_name, u.last_name,
	u.display_name, u.avatar_url, u.primary_role, u.status,
	u.vehicle_type, u.vehicle_registration_number, u.vehicle_details, u.insurance_details,
	u.max_carry_capacity_kg, u.license_number, u.license_expiry,
	u.is_available, u.on_trip, u.current_lat, u.current_lng, u.last_location_update,
	u.kyc_verified, u.kyc_data, u.verification_docs,
	u.bank_details, u.payout_methods,
	u.rating_avg, u.rating_count, u.total_deliveries, u.total_orders, u.earnings,
	u.created_at, u.updated_at
`

// scanRider scans a row into a User model.
func scanRider(row interface{ Scan(...interface{}) error }) (*models.User, error) {
	var u models.User
	var licenseExpiry sql.NullString
	err := row.Scan(
		&u.ID, &u.UserID, &u.Phone, &u.Email, &u.FirstName, &u.LastName,
		&u.DisplayName, &u.AvatarURL, &u.PrimaryRole, &u.Status,
		&u.VehicleType, &u.VehicleRegistrationNumber, &u.VehicleDetails, &u.InsuranceDetails,
		&u.MaxCarryCapacityKg, &u.LicenseNumber, &licenseExpiry,
		&u.IsAvailable, &u.OnTrip, &u.CurrentLat, &u.CurrentLng, &u.LastLocationUpdate,
		&u.KYCVerified, &u.KYCData, &u.VerificationDocs,
		&u.BankDetails, &u.PayoutMethods,
		&u.RatingAvg, &u.RatingCount, &u.TotalDeliveries, &u.TotalOrders, &u.Earnings,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if licenseExpiry.Valid {
		u.LicenseExpiry = &licenseExpiry.String
	}
	return &u, nil
}

// GetByID fetches a rider (delivery_driver) by UUID from users table.
func (r *RiderRepository) GetByID(ctx context.Context, userID string) (*models.User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users u WHERE u.id = $1 AND u.primary_role = 'delivery_driver'`, riderSelectColumns)
	row := r.db.QueryRowContext(ctx, query, userID)
	return scanRider(row)
}

// UpdateProfile updates basic profile fields.
func (r *RiderRepository) UpdateProfile(ctx context.Context, userID string, firstName, lastName, email, displayName, avatarURL *string) (*models.User, error) {
	query := `UPDATE users SET
		first_name = COALESCE($2, first_name),
		last_name = COALESCE($3, last_name),
		email = COALESCE($4, email),
		display_name = COALESCE($5, display_name),
		avatar_url = COALESCE($6, avatar_url),
		updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'`
	_, err := r.db.ExecContext(ctx, query, userID, firstName, lastName, email, displayName, avatarURL)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, userID)
}

// UpdateVehicle updates vehicle-related fields.
func (r *RiderRepository) UpdateVehicle(ctx context.Context, userID string, vehicleType, regNumber, vehicleDetails, insuranceDetails *string, maxCapacity *float64, licenseNumber, licenseExpiry *string) (*models.User, error) {
	query := `UPDATE users SET
		vehicle_type = COALESCE($2, vehicle_type),
		vehicle_registration_number = COALESCE($3, vehicle_registration_number),
		vehicle_details = COALESCE($4::jsonb, vehicle_details),
		insurance_details = COALESCE($5::jsonb, insurance_details),
		max_carry_capacity_kg = COALESCE($6, max_carry_capacity_kg),
		license_number = COALESCE($7, license_number),
		license_expiry = COALESCE($8::date, license_expiry),
		updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'`
	_, err := r.db.ExecContext(ctx, query, userID, vehicleType, regNumber, vehicleDetails, insuranceDetails, maxCapacity, licenseNumber, licenseExpiry)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, userID)
}

// UpdateBankDetails updates bank/payout fields.
func (r *RiderRepository) UpdateBankDetails(ctx context.Context, userID string, bankDetails, payoutMethods *string) (*models.User, error) {
	query := `UPDATE users SET
		bank_details = COALESCE($2::jsonb, bank_details),
		payout_methods = COALESCE($3::jsonb, payout_methods),
		updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'`
	_, err := r.db.ExecContext(ctx, query, userID, bankDetails, payoutMethods)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, userID)
}

// UpdateKYC updates KYC-related fields.
func (r *RiderRepository) UpdateKYC(ctx context.Context, userID string, licenseNumber, kycData, verificationDocs *string) (*models.User, error) {
	query := `UPDATE users SET
		license_number = COALESCE($2, license_number),
		kyc_data = COALESCE($3::jsonb, kyc_data),
		verification_docs = COALESCE($4::jsonb, verification_docs),
		updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'`
	_, err := r.db.ExecContext(ctx, query, userID, licenseNumber, kycData, verificationDocs)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, userID)
}

// SetAvailability updates is_available flag.
func (r *RiderRepository) SetAvailability(ctx context.Context, userID string, available bool) error {
	query := `UPDATE users SET is_available = $2, updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'`
	_, err := r.db.ExecContext(ctx, query, userID, available)
	return err
}

// SetOnTrip updates on_trip flag.
func (r *RiderRepository) SetOnTrip(ctx context.Context, userID string, onTrip bool) error {
	query := `UPDATE users SET on_trip = $2, updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'`
	_, err := r.db.ExecContext(ctx, query, userID, onTrip)
	return err
}

// SetOnTripTx updates on_trip flag within a transaction.
func (r *RiderRepository) SetOnTripTx(ctx context.Context, tx *sql.Tx, userID string, onTrip bool) error {
	query := `UPDATE users SET on_trip = $2, updated_at = NOW() WHERE id = $1`
	_, err := tx.ExecContext(ctx, query, userID, onTrip)
	return err
}

// UpdateLocation updates current lat/lng in users table and returns the last update time and availability.
func (r *RiderRepository) UpdateLocation(ctx context.Context, userID string, lat, lng float64) (*time.Time, bool, error) {
	query := `UPDATE users SET current_lat = $2, current_lng = $3, last_location_update = NOW(), updated_at = NOW()
		WHERE id = $1 AND primary_role = 'delivery_driver'
		RETURNING last_location_update, is_available`
	
	var lastUpdate time.Time
	var isAvailable bool
	err := r.db.QueryRowContext(ctx, query, userID, lat, lng).Scan(&lastUpdate, &isAvailable)
	if err != nil {
		return nil, false, err
	}
	return &lastUpdate, isAvailable, nil
}

// IncrementDeliveryCount increments total_deliveries and adds to earnings.
func (r *RiderRepository) IncrementDeliveryCount(ctx context.Context, tx *sql.Tx, userID string, earningsAmount float64) error {
	query := `UPDATE users SET
		total_deliveries = total_deliveries + 1,
		earnings = earnings + $2,
		on_trip = false,
		updated_at = NOW()
		WHERE id = $1`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, userID, earningsAmount)
	} else {
		_, err = r.db.ExecContext(ctx, query, userID, earningsAmount)
	}
	return err
}

// GetAvailability returns current is_available and on_trip state.
func (r *RiderRepository) GetAvailability(ctx context.Context, userID string) (bool, bool, error) {
	var available, onTrip bool
	query := `SELECT is_available, on_trip FROM users WHERE id = $1 AND primary_role = 'delivery_driver'`
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&available, &onTrip)
	return available, onTrip, err
}

// GetCurrentLocation returns current lat/lng from users table.
func (r *RiderRepository) GetCurrentLocation(ctx context.Context, userID string) (*float64, *float64, *time.Time, error) {
	var lat, lng *float64
	var lastUpdate *time.Time
	query := `SELECT current_lat, current_lng, last_location_update FROM users WHERE id = $1`
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&lat, &lng, &lastUpdate)
	return lat, lng, lastUpdate, err
}

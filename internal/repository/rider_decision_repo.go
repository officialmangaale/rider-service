package repository

import (
	"context"
	"database/sql"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
)

// riderDecisionSQL evaluates one rider against each FindNearestRiders filter
// separately, so a trace can say which filter excluded them. Read-only; it
// returns booleans, an age in seconds and a distance, never coordinates.
// It runs only for an active, targeted dispatch trace.
const riderDecisionSQL = `
	SELECT
		ra.rider_id IS NOT NULL                                        AS availability_row,
		COALESCE(ra.is_online, FALSE)                                  AS online,
		COALESCE(ra.is_available, FALSE)                               AS available,
		ra.rider_id IS NOT NULL AND ra.current_order_id IS NULL        AS idle,
		rl.rider_id IS NOT NULL                                        AS location_row,
		COALESCE(rl.last_updated_at <= NOW() AND rl.last_updated_at >= NOW() - make_interval(secs =>
            (SELECT location_max_age_seconds FROM online_delivery_dispatch_config WHERE singleton)), FALSE) AS location_fresh,
		EXTRACT(EPOCH FROM NOW() - rl.last_updated_at)::bigint         AS location_age_s,
		CASE WHEN rl.rider_id IS NULL THEN NULL ELSE
			6371 * acos(LEAST(1.0, GREATEST(-1.0, cos(radians($2)) * cos(radians(rl.latitude))
				* cos(radians(rl.longitude) - radians($3))
				+ sin(radians($2)) * sin(radians(rl.latitude)))))
		END AS distance_km,
        CASE WHEN u.id IS NULL OR COALESCE(u.is_deleted,false) OR COALESCE(u.status,'active')<>'active'
                   OR u.primary_role IS NULL OR u.primary_role NOT IN ('rider','delivery_driver') THEN 'rider_account_ineligible'
             WHEN COALESCE(ra.is_deleted,false) OR COALESCE(rl.is_deleted,false) THEN 'rider_state_deleted'
             WHEN rl.rider_id IS NOT NULL AND NOT COALESCE(rl.latitude BETWEEN -90 AND 90 AND rl.longitude BETWEEN -180 AND 180,false) THEN 'rider_location_invalid'
             WHEN rl.last_updated_at > NOW() THEN 'rider_location_in_future'
             ELSE '' END AS policy_failure
	FROM (SELECT $1::text AS rider_id) target
	LEFT JOIN rider_availability ra ON ra.rider_id = target.rider_id
	LEFT JOIN rider_locations rl ON rl.rider_id = target.rider_id
    LEFT JOIN users u ON u.id::text=target.rider_id`

// RiderDecisionVector reports, for one rider, the result of each dispatch
// filter against a pickup point.
func (r *DeliveryRepository) RiderDecisionVector(ctx context.Context, riderID string, pickupLat, pickupLng, radiusKm float64) (dispatchtrace.RiderDecision, error) {
	var d dispatchtrace.RiderDecision
	var age sql.NullInt64
	var distance sql.NullFloat64
	err := r.db.QueryRowContext(ctx, riderDecisionSQL, riderID, pickupLat, pickupLng).Scan(
		&d.AvailabilityRow, &d.Online, &d.Available, &d.Idle,
		&d.LocationRow, &d.LocationFresh, &age, &distance, &d.PolicyFailure,
	)
	if err != nil {
		return d, err
	}
	if age.Valid {
		v := age.Int64
		d.LocationAgeSec = &v
	}
	if distance.Valid {
		v := distance.Float64
		d.DistanceKm = &v
		d.WithinRadius = v <= radiusKm
	}
	return d, nil
}

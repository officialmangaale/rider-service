package repository

import (
	"context"
	"time"
)

// Re-dispatch of unmatched platform orders.
//
// Dispatch runs once, when restaurant-service publishes the order. If no rider
// is eligible at that instant — typically because the only nearby rider's GPS
// was a few minutes old — the order is marked no_rider_found and, until now,
// nothing offered it again: the rider whose location refreshed seconds later
// never saw it. As of 2026-09-11 every platform delivery order in production
// had ended that way (order 13286 is the one investigated).
//
// These queries let a worker re-offer such orders while the restaurant still
// wants a rider.

// RedispatchCandidate is a platform delivery order that may be offered again.
type RedispatchCandidate struct {
	DeliveryOrderID int
	OrderID         int
}

// redispatchCandidateSQL selects platform delivery orders that are unassigned,
// have no live or recently expired offer, and whose restaurant order is still
// waiting for a rider.
//
// Every guard matters:
//   - restaurant_owned / assigned columns: a restaurant's own rider or a
//     platform rider already has it.
//   - orders.order_status: the restaurant-service dispatch gate. A cancelled,
//     delivered or completed order is never offered, even though its
//     delivery_orders row still says no_rider_found.
//   - created_at >= NOW() - maxAge: production holds orders from months ago
//     that were never closed and still read "confirmed". Without this ceiling
//     riders would be rung for them.
//   - the NOT EXISTS: an order with a pending offer is left alone, and one
//     whose last offer expired less than cooldown ago waits, so a rider who
//     let an offer lapse is not rung again immediately.
const redispatchCandidateSQL = `
		SELECT d.delivery_order_id, d.order_id
		FROM delivery_orders d
		JOIN orders o ON o.order_id = d.order_id
		WHERE d.is_deleted = FALSE
		  AND d.delivery_status IN ('pending', 'rider_searching', 'no_rider_found')
		  AND d.assigned_rider_id IS NULL
		  AND COALESCE(d.rider_user_id, '') = ''
		  AND COALESCE(d.restaurant_owned, FALSE) = FALSE
		  AND d.created_at >= NOW() - make_interval(secs => $1)
		  AND o.is_deleted = FALSE
		  AND LOWER(o.order_status) IN ('accepted', 'confirmed', 'preparing', 'ready')
		  AND COALESCE(o.assigned_rider_user_id, '') = ''
		  AND NOT EXISTS (
		        SELECT 1 FROM delivery_order_requests r
		         WHERE r.delivery_order_id = d.delivery_order_id
		           AND (r.status = 'accepted'
		                OR r.expires_at > NOW() - make_interval(secs => $2)))
		ORDER BY d.created_at ASC
		LIMIT $3`

// FindRedispatchCandidates returns up to limit orders due for another offer.
func (r *DeliveryRepository) FindRedispatchCandidates(ctx context.Context, maxAge, cooldown time.Duration, limit int) ([]RedispatchCandidate, error) {
	rows, err := r.db.QueryContext(ctx, redispatchCandidateSQL, maxAge.Seconds(), cooldown.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RedispatchCandidate
	for rows.Next() {
		var c RedispatchCandidate
		if err := rows.Scan(&c.DeliveryOrderID, &c.OrderID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeclinedRiderIDs returns the riders who explicitly declined this delivery
// order. They are never offered it again: a decline is that rider's answer.
// A rider whose offer merely expired is not in this set and may be re-offered.
func (r *DeliveryRepository) DeclinedRiderIDs(ctx context.Context, deliveryOrderID int) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rider_id FROM delivery_order_requests
		 WHERE delivery_order_id = $1 AND status = 'rejected'`,
		deliveryOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	declined := map[string]bool{}
	for rows.Next() {
		var riderID string
		if err := rows.Scan(&riderID); err != nil {
			return nil, err
		}
		declined[riderID] = true
	}
	return declined, rows.Err()
}

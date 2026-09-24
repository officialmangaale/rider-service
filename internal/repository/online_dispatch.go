package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// WithdrawFoodDelivery is an explicit pre-pickup transition, not customer
// cancellation. The old rider's offer stays rejected, so they are not rung again.
func (r *DeliveryRepository) WithdrawFoodDelivery(ctx context.Context, orderID int, riderID, reason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = r.LockFoodOrder(ctx, tx, orderID); err != nil {
		return err
	}
	var id int
	var status string
	var assigned sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT delivery_order_id,delivery_status,assigned_rider_id FROM delivery_orders
        WHERE order_id=$1 AND order_type='food' FOR UPDATE`, orderID).Scan(&id, &status, &assigned)
	if err != nil {
		return err
	}
	if !assigned.Valid || assigned.String != riderID {
		return fmt.Errorf("order is no longer assigned to this rider")
	}
	if status != "rider_assigned" && status != "rider_arrived_restaurant" {
		return fmt.Errorf("cannot withdraw after pickup")
	}
	result, err := tx.ExecContext(ctx, `UPDATE orders SET assigned_rider_user_id=NULL, delivery_partner_id=NULL,
        rider_id=NULL, assigned_rider_name=NULL,assigned_rider_phone=NULL,rider_name=NULL,rider_phone=NULL,
        assigned_at=NULL,rider_assigned_at=NULL,delivery_status='rider_searching',updated_at=NOW()
        WHERE order_id=$1 AND assigned_rider_user_id=$2 AND order_status IN ('preparing','ready')
          AND picked_up_at IS NULL AND delivered_at IS NULL`, orderID, riderID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("order can no longer be withdrawn")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE delivery_orders SET assigned_rider_id=NULL,rider_user_id=NULL,
        assigned_at=NULL,delivery_status='rider_searching',updated_at=NOW() WHERE delivery_order_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE delivery_order_requests SET status=CASE WHEN rider_id=$2 THEN 'rejected' ELSE 'cancelled' END,
        expires_at=NOW(),updated_at=NOW() WHERE delivery_order_id=$1`, id, riderID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE rider_availability SET current_order_id=NULL,is_available=is_online,updated_at=NOW()
        WHERE rider_id=$1 AND current_order_id=$2`, riderID, orderID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO delivery_status_history(order_id,from_status,to_status,changed_by,metadata)
        VALUES($1,$2,'rider_searching',$3,jsonb_build_object('reason',$4::text))`, orderID, status, riderID, reason); err != nil {
		return err
	}
	return tx.Commit()
}

const onlineFoodPredicate = `mangaale_online_delivery(o.order_type::text, o.is_qrunch,
    o.metadata, o.creation_source, o.dining_session_id, o.counter_id)`

func (r *DeliveryRepository) OwnAssignmentEventCurrent(ctx context.Context, orderID int, riderID string) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM orders o WHERE o.order_id=$1
        AND o.assigned_rider_user_id=$2 AND NOT o.is_deleted
        AND o.order_status NOT IN ('cancelled','rejected','completed','delivered')
        AND NOT EXISTS(SELECT 1 FROM delivery_orders d WHERE d.order_type='food' AND d.order_id=o.order_id
            AND NOT COALESCE(d.restaurant_owned,false)))`, orderID, riderID).Scan(&allowed)
	return allowed, err
}

type FoodProgressRepair struct {
	DeliveryOrderID int
	RiderID, Target string
}

func (r *DeliveryRepository) FoodProgressRepairs(ctx context.Context) ([]FoodProgressRepair, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT d.delivery_order_id,d.assigned_rider_id,
        CASE WHEN o.order_status='out_for_delivery' THEN 'picked_up' ELSE 'delivered' END
        FROM delivery_orders d JOIN orders o ON o.order_id=d.order_id
        WHERE d.order_type='food' AND NOT d.is_deleted AND NOT o.is_deleted
          AND d.assigned_rider_id=o.assigned_rider_user_id AND `+onlineFoodPredicate+`
          AND ((o.order_status='out_for_delivery' AND d.delivery_status IN ('rider_assigned','rider_arrived_restaurant'))
            OR ((o.order_status='delivered' OR (o.order_status='completed' AND o.delivered_at IS NOT NULL))
              AND d.delivery_status IN ('rider_assigned','rider_arrived_restaurant','picked_up','on_the_way')))
        ORDER BY d.updated_at LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repairs []FoodProgressRepair
	for rows.Next() {
		var repair FoodProgressRepair
		if err := rows.Scan(&repair.DeliveryOrderID, &repair.RiderID, &repair.Target); err != nil {
			return nil, err
		}
		repairs = append(repairs, repair)
	}
	return repairs, rows.Err()
}

func (r *DeliveryRepository) ExpireFoodSearches(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE orders SET delivery_status='dispatch_expired',updated_at=NOW()
        WHERE order_id IN (SELECT o.order_id FROM orders o WHERE `+onlineFoodPredicate+`
          AND NOT o.is_deleted AND o.order_status IN ('preparing','ready')
          AND COALESCE(o.assigned_rider_user_id,'')='' AND COALESCE(o.delivery_partner_id,'')=''
          AND o.rider_id IS NULL AND COALESCE(o.delivery_status,'')<>'dispatch_expired'
          AND o.created_at < NOW()-make_interval(secs => (SELECT search_max_age_seconds FROM online_delivery_dispatch_config WHERE singleton))
          AND EXISTS (SELECT 1 FROM delivery_orders d WHERE d.order_type='food' AND d.order_id=o.order_id
              AND d.assigned_rider_id IS NULL AND d.delivery_status IN ('rider_searching','no_rider_found'))
          ORDER BY o.created_at LIMIT 50)
        AND order_status IN ('preparing','ready') AND COALESCE(assigned_rider_user_id,'')=''
        AND COALESCE(delivery_partner_id,'')='' AND rider_id IS NULL`)
	return err
}

const offerableFoodPredicate = onlineFoodPredicate + `
    AND mangaale_dispatch_enabled(o.restaurant_id)
    AND o.is_deleted = false
    AND lower(o.order_status::text) IN ('preparing', 'ready')
    AND COALESCE(o.assigned_rider_user_id, '') = ''
    AND COALESCE(o.delivery_partner_id, '') = '' AND o.rider_id IS NULL
    AND o.picked_up_at IS NULL AND o.delivered_at IS NULL
    AND o.created_at >= NOW() - make_interval(secs =>
        (SELECT search_max_age_seconds FROM online_delivery_dispatch_config WHERE singleton))`

// FoodDispatchAllowed always reads the canonical order, never trusts a queued
// event's type/address/status. Grocery keeps its separate existing policy.
func (r *DeliveryRepository) FoodDispatchAllowed(ctx context.Context, orderID int) (bool, error) {
	var allowed bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM orders o WHERE o.order_id=$1 AND `+offerableFoodPredicate+`)`, orderID).Scan(&allowed)
	return allowed, err
}

// LockFoodOrder precedes the delivery/request/rider locks everywhere in the new
// flow. Restaurant cancellation updates this same row, serializing with accept.
func (r *DeliveryRepository) LockFoodOrder(ctx context.Context, tx *sql.Tx, orderID int) error {
	var id int
	return tx.QueryRowContext(ctx, `SELECT order_id FROM orders WHERE order_id=$1 FOR UPDATE`, orderID).Scan(&id)
}

func (r *DeliveryRepository) ClaimFoodOrder(ctx context.Context, tx *sql.Tx, orderID int, riderID string, radius float64) error {
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM orders o WHERE o.order_id=$1 AND `+offerableFoodPredicate+`)`, orderID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("order is no longer available for acceptance")
	}

	// Lock availability before rechecking the rider. This serializes attempts
	// on different orders, including go-offline and availability updates.
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT rider_id FROM rider_availability WHERE rider_id=$1 FOR UPDATE`, riderID).Scan(&id); err != nil {
		return fmt.Errorf("rider is unavailable")
	}
	var eligible bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(
        SELECT 1 FROM rider_availability ra
        JOIN rider_locations rl ON rl.rider_id=ra.rider_id
        JOIN users u ON u.id::text=ra.rider_id
        JOIN orders o ON o.order_id=$2
        JOIN restaurants rest ON rest.restaurant_id=o.restaurant_id
        WHERE ra.rider_id=$1 AND ra.is_online AND ra.is_available AND ra.current_order_id IS NULL
          AND NOT COALESCE(ra.is_deleted,false) AND NOT COALESCE(rl.is_deleted,false)
          AND NOT COALESCE(u.is_deleted,false) AND COALESCE(u.status,'active')='active'
          AND u.primary_role IN ('rider','delivery_driver')
          AND rl.latitude BETWEEN -90 AND 90 AND rl.longitude BETWEEN -180 AND 180
          AND rest.latitude BETWEEN -90 AND 90 AND rest.longitude BETWEEN -180 AND 180
          AND rl.last_updated_at <= NOW()
          AND rl.last_updated_at >= NOW() - make_interval(secs =>
              (SELECT location_max_age_seconds FROM online_delivery_dispatch_config WHERE singleton))
          AND 6371*acos(LEAST(1.0,GREATEST(-1.0,
              cos(radians(rest.latitude))*cos(radians(rl.latitude))*cos(radians(rl.longitude)-radians(rest.longitude))
              +sin(radians(rest.latitude))*sin(radians(rl.latitude))))) <= $3
          AND NOT EXISTS (SELECT 1 FROM delivery_orders d WHERE
              (d.assigned_rider_id=$1 OR d.rider_user_id=$1)
              AND d.delivery_status NOT IN ('delivered','cancelled') AND NOT d.is_deleted)
    )`, riderID, orderID, radius).Scan(&eligible)
	if err != nil {
		return err
	}
	if !eligible {
		return fmt.Errorf("rider is offline, busy, or outside the pickup area")
	}
	// Persist the owner/customer projection in the very same commit as the
	// assignment. HTTP and socket notification failure cannot lose it.
	_, err = tx.ExecContext(ctx, `UPDATE orders SET assigned_rider_user_id=$2,
        delivery_partner_id=$2, assigned_at=NOW(), rider_assigned_at=NOW(),
        rider_name=COALESCE(u.display_name,u.first_name,''), rider_phone=u.phone,
        assigned_rider_name=COALESCE(u.display_name,u.first_name,''), assigned_rider_phone=u.phone,
        rider_vehicle_type=u.vehicle_type, rider_vehicle_number=u.vehicle_registration_number,
        delivery_status='rider_assigned', updated_at=NOW()
        FROM users u WHERE orders.order_id=$1 AND u.id::text=$2`, orderID, riderID)
	return err
}

// RefreshFoodEvent removes stale/forged coordinates and private details from
// queue authority. It also supplies reconciliation after a missed SQS publish.
func (r *DeliveryRepository) RefreshFoodEvent(ctx context.Context, evt *models.OrderPlacedEvent) error {
	return r.db.QueryRowContext(ctx, `SELECT o.restaurant_id,
        COALESCE(rest.name,''), COALESCE(rest.metadata->>'restaurant_phone',rest.metadata->>'phone',''),
        rest.latitude, rest.longitude, COALESCE(rest.street_address,''),
        COALESCE(o.delivery_latitude,0), COALESCE(o.delivery_longitude,0), COALESCE(o.delivery_address,''),
        o.total_amount, COALESCE(o.pay_by::text,'cash'),
        COALESCE(o.metadata->>'customer_phone',''), COALESCE(o.metadata->>'customer_name','')
        FROM orders o JOIN restaurants rest ON rest.restaurant_id=o.restaurant_id
        WHERE o.order_id=$1 AND `+offerableFoodPredicate+`
        AND rest.latitude BETWEEN -90 AND 90 AND rest.longitude BETWEEN -180 AND 180`, evt.OrderID).
		Scan(&evt.RestaurantID, &evt.RestaurantName, &evt.RestaurantPhone,
			&evt.Pickup.Latitude, &evt.Pickup.Longitude, &evt.Pickup.Address,
			&evt.Drop.Latitude, &evt.Drop.Longitude, &evt.Drop.Address, &evt.Amount, &evt.PaymentMode,
			&evt.CustomerPhone, &evt.CustomerName)
}

func (r *DeliveryRepository) MissingFoodDispatches(ctx context.Context, limit int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT o.order_id FROM orders o WHERE `+offerableFoodPredicate+`
        AND NOT EXISTS(SELECT 1 FROM delivery_orders d WHERE d.order_id=o.order_id AND d.order_type='food')
        ORDER BY o.created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *DeliveryRepository) RequestForLegacyAssignment(ctx context.Context, orderID int, riderID string) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT r.request_id FROM delivery_order_requests r
        JOIN delivery_orders d ON d.delivery_order_id=r.delivery_order_id
        WHERE d.order_type='food' AND d.order_id=$1 AND r.rider_id=$2`, orderID, riderID).Scan(&id)
	return id, err
}

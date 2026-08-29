package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// DeliveryRepository handles all delivery-flow tables.
type DeliveryRepository struct {
	db *sql.DB
}

type RiderEligibilitySummary struct {
	OnlineRiders       int
	AvailableRiders    int
	RidersWithLocation int
	RidersWithFreshGPS int
	RidersWithinRadius int
}

func NewDeliveryRepository(db *sql.DB) *DeliveryRepository {
	return &DeliveryRepository{db: db}
}

// ===== PROCESSED EVENTS (Idempotency) =====

func (r *DeliveryRepository) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id=$1)`, eventID).Scan(&exists)
	return exists, err
}

func (r *DeliveryRepository) IsOrderProcessed(ctx context.Context, orderID int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM processed_events WHERE order_id=$1)`, orderID).Scan(&exists)
	return exists, err
}

func (r *DeliveryRepository) MarkEventProcessed(ctx context.Context, eventID string, orderID int, eventType string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, order_id, event_type) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		eventID, orderID, eventType)
	return err
}

// ===== DELIVERY ORDERS =====

func (r *DeliveryRepository) CreateDeliveryOrder(ctx context.Context, evt *models.OrderPlacedEvent) (*models.DeliveryOrder, error) {
	var o models.DeliveryOrder
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO delivery_orders (order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assignment_type, restaurant_name, restaurant_phone, customer_name, customer_phone)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'rider_searching','platform',$12,$13,$14,$15)
		 ON CONFLICT (order_id) DO UPDATE SET
			restaurant_name = COALESCE(NULLIF(delivery_orders.restaurant_name, ''), EXCLUDED.restaurant_name),
			restaurant_phone = COALESCE(NULLIF(delivery_orders.restaurant_phone, ''), EXCLUDED.restaurant_phone),
			customer_name = COALESCE(NULLIF(delivery_orders.customer_name, ''), EXCLUDED.customer_name),
			customer_phone = COALESCE(NULLIF(delivery_orders.customer_phone, ''), EXCLUDED.customer_phone),
			pickup_latitude = CASE WHEN delivery_orders.pickup_latitude = 0 THEN EXCLUDED.pickup_latitude ELSE delivery_orders.pickup_latitude END,
			pickup_longitude = CASE WHEN delivery_orders.pickup_longitude = 0 THEN EXCLUDED.pickup_longitude ELSE delivery_orders.pickup_longitude END,
			pickup_address = COALESCE(NULLIF(delivery_orders.pickup_address, ''), EXCLUDED.pickup_address),
			drop_latitude = CASE WHEN delivery_orders.drop_latitude = 0 THEN EXCLUDED.drop_latitude ELSE delivery_orders.drop_latitude END,
			drop_longitude = CASE WHEN delivery_orders.drop_longitude = 0 THEN EXCLUDED.drop_longitude ELSE delivery_orders.drop_longitude END,
			drop_address = COALESCE(NULLIF(delivery_orders.drop_address, ''), EXCLUDED.drop_address),
			amount = CASE WHEN delivery_orders.amount = 0 THEN EXCLUDED.amount ELSE delivery_orders.amount END,
			payment_mode = COALESCE(NULLIF(delivery_orders.payment_mode, ''), EXCLUDED.payment_mode)
		 RETURNING delivery_order_id, order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, created_at, updated_at, assigned_at, picked_up_at, delivered_at, rider_user_id, assignment_type, restaurant_owned, restaurant_name, restaurant_phone, customer_name, customer_phone, items_summary, rider_arrived_at`,
		evt.OrderID, evt.RestaurantID, evt.CustomerID.Int(),
		evt.Pickup.Latitude, evt.Pickup.Longitude, evt.Pickup.Address,
		evt.Drop.Latitude, evt.Drop.Longitude, evt.Drop.Address,
		evt.Amount, evt.PaymentMode, evt.RestaurantName, evt.RestaurantPhone,
		evt.CustomerName, evt.CustomerPhone,
	).Scan(&o.DeliveryOrderID, &o.OrderID, &o.RestaurantID, &o.CustomerID,
		&o.PickupLatitude, &o.PickupLongitude, &o.PickupAddress,
		&o.DropLatitude, &o.DropLongitude, &o.DropAddress,
		&o.Amount, &o.PaymentMode, &o.DeliveryStatus, &o.AssignedRiderID,
		&o.CreatedAt, &o.UpdatedAt, &o.AssignedAt, &o.PickedUpAt, &o.DeliveredAt,
		&o.RiderUserID, &o.AssignmentType, &o.RestaurantOwned, &o.RestaurantName, &o.RestaurantPhone,
		&o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.RiderArrivedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *DeliveryRepository) UpsertRestaurantOwnedOrder(ctx context.Context, evt *models.RiderAssignedToOrderEvent) (*models.DeliveryOrder, error) {
	var o models.DeliveryOrder
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO delivery_orders (
			order_id, restaurant_id, customer_id,
			pickup_latitude, pickup_longitude, pickup_address,
			drop_latitude, drop_longitude, drop_address,
			amount, payment_mode,
			rider_user_id, assignment_type, restaurant_owned,
			restaurant_name, restaurant_phone,
			delivery_status, assigned_at, assigned_rider_id
		 )
		 SELECT
			o.order_id,
			COALESCE(o.restaurant_id, $2),
			COALESCE(o.customer_id, 0),
			COALESCE(rest.latitude, 0),
			COALESCE(rest.longitude, 0),
			COALESCE(rest.street_address, ''),
			COALESCE(o.delivery_latitude, 0),
			COALESCE(o.delivery_longitude, 0),
			COALESCE(o.delivery_address, ''),
			COALESCE(o.total_amount, 0),
			COALESCE(o.pay_by::text, 'cash'),
			$3,
			'restaurant_owned',
			true,
			COALESCE(NULLIF($4, ''), rest.name, 'Restaurant'),
			COALESCE(NULLIF($5, ''), owner_user.business_phone, owner_user.phone, ''),
			'rider_assigned',
			$6,
			$3
		 FROM orders o
		 LEFT JOIN restaurants rest ON rest.restaurant_id = o.restaurant_id
		 LEFT JOIN users owner_user ON owner_user.id = rest.user_id
		 WHERE o.order_id = $1
		 ON CONFLICT (order_id) DO UPDATE SET
			rider_user_id = EXCLUDED.rider_user_id,
			assignment_type = 'restaurant_owned',
			restaurant_owned = true,
			restaurant_name = COALESCE(NULLIF(delivery_orders.restaurant_name, ''), EXCLUDED.restaurant_name),
			restaurant_phone = COALESCE(NULLIF(delivery_orders.restaurant_phone, ''), EXCLUDED.restaurant_phone),
			pickup_latitude = CASE WHEN delivery_orders.pickup_latitude = 0 THEN EXCLUDED.pickup_latitude ELSE delivery_orders.pickup_latitude END,
			pickup_longitude = CASE WHEN delivery_orders.pickup_longitude = 0 THEN EXCLUDED.pickup_longitude ELSE delivery_orders.pickup_longitude END,
			pickup_address = COALESCE(NULLIF(delivery_orders.pickup_address, ''), EXCLUDED.pickup_address),
			drop_latitude = CASE WHEN delivery_orders.drop_latitude = 0 THEN EXCLUDED.drop_latitude ELSE delivery_orders.drop_latitude END,
			drop_longitude = CASE WHEN delivery_orders.drop_longitude = 0 THEN EXCLUDED.drop_longitude ELSE delivery_orders.drop_longitude END,
			drop_address = COALESCE(NULLIF(delivery_orders.drop_address, ''), EXCLUDED.drop_address),
			amount = CASE WHEN delivery_orders.amount = 0 THEN EXCLUDED.amount ELSE delivery_orders.amount END,
			payment_mode = COALESCE(NULLIF(delivery_orders.payment_mode, ''), EXCLUDED.payment_mode),
			delivery_status = 'rider_assigned',
			assigned_at = EXCLUDED.assigned_at,
			assigned_rider_id = EXCLUDED.rider_user_id,
			updated_at = NOW()
		 RETURNING delivery_order_id, order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, created_at, updated_at, assigned_at, picked_up_at, delivered_at, rider_user_id, assignment_type, restaurant_owned, restaurant_name, restaurant_phone, customer_name, customer_phone, items_summary, rider_arrived_at`,
		evt.OrderID, evt.RestaurantID, evt.RiderUserID, evt.RestaurantName, evt.RestaurantPhone, evt.AssignedAt,
	).Scan(&o.DeliveryOrderID, &o.OrderID, &o.RestaurantID, &o.CustomerID,
		&o.PickupLatitude, &o.PickupLongitude, &o.PickupAddress,
		&o.DropLatitude, &o.DropLongitude, &o.DropAddress,
		&o.Amount, &o.PaymentMode, &o.DeliveryStatus, &o.AssignedRiderID,
		&o.CreatedAt, &o.UpdatedAt, &o.AssignedAt, &o.PickedUpAt, &o.DeliveredAt,
		&o.RiderUserID, &o.AssignmentType, &o.RestaurantOwned, &o.RestaurantName, &o.RestaurantPhone,
		&o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.RiderArrivedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *DeliveryRepository) GetDeliveryOrderByOrderID(ctx context.Context, orderID int) (*models.DeliveryOrder, error) {
	var o models.DeliveryOrder
	err := r.db.QueryRowContext(ctx,
		`SELECT delivery_order_id, order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, created_at, updated_at, assigned_at, picked_up_at, delivered_at, rider_user_id, assignment_type, restaurant_owned, restaurant_name, restaurant_phone, customer_name, customer_phone, items_summary, rider_arrived_at
		 FROM delivery_orders WHERE order_id=$1`, orderID,
	).Scan(&o.DeliveryOrderID, &o.OrderID, &o.RestaurantID, &o.CustomerID,
		&o.PickupLatitude, &o.PickupLongitude, &o.PickupAddress,
		&o.DropLatitude, &o.DropLongitude, &o.DropAddress,
		&o.Amount, &o.PaymentMode, &o.DeliveryStatus, &o.AssignedRiderID,
		&o.CreatedAt, &o.UpdatedAt, &o.AssignedAt, &o.PickedUpAt, &o.DeliveredAt,
		&o.RiderUserID, &o.AssignmentType, &o.RestaurantOwned, &o.RestaurantName, &o.RestaurantPhone,
		&o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.RiderArrivedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *DeliveryRepository) GetDeliveryOrderByID(ctx context.Context, deliveryOrderID int) (*models.DeliveryOrder, error) {
	var o models.DeliveryOrder
	err := r.db.QueryRowContext(ctx,
		`SELECT delivery_order_id, order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, created_at, updated_at, assigned_at, picked_up_at, delivered_at, rider_user_id, assignment_type, restaurant_owned, restaurant_name, restaurant_phone, customer_name, customer_phone, items_summary, rider_arrived_at
		 FROM delivery_orders WHERE delivery_order_id=$1`, deliveryOrderID,
	).Scan(&o.DeliveryOrderID, &o.OrderID, &o.RestaurantID, &o.CustomerID,
		&o.PickupLatitude, &o.PickupLongitude, &o.PickupAddress,
		&o.DropLatitude, &o.DropLongitude, &o.DropAddress,
		&o.Amount, &o.PaymentMode, &o.DeliveryStatus, &o.AssignedRiderID,
		&o.CreatedAt, &o.UpdatedAt, &o.AssignedAt, &o.PickedUpAt, &o.DeliveredAt,
		&o.RiderUserID, &o.AssignmentType, &o.RestaurantOwned, &o.RestaurantName, &o.RestaurantPhone,
		&o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.RiderArrivedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *DeliveryRepository) GetRiderOrders(ctx context.Context, riderUserID string, statuses []string) ([]*models.DeliveryOrder, error) {
	// Build the IN clause for statuses
	if len(statuses) == 0 {
		return []*models.DeliveryOrder{}, nil
	}
	statusArgs := ""
	args := []interface{}{riderUserID}
	for i, s := range statuses {
		if i > 0 {
			statusArgs += ","
		}
		statusArgs += fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	query := fmt.Sprintf(`SELECT delivery_order_id, order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, created_at, updated_at, assigned_at, picked_up_at, delivered_at, rider_user_id, assignment_type, restaurant_owned, restaurant_name, restaurant_phone, customer_name, customer_phone, items_summary, rider_arrived_at
		 FROM delivery_orders
		 WHERE (rider_user_id=$1 OR assigned_rider_id=$1)
		 AND delivery_status IN (%s)
		 ORDER BY created_at DESC`, statusArgs)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*models.DeliveryOrder
	for rows.Next() {
		var o models.DeliveryOrder
		if err := rows.Scan(&o.DeliveryOrderID, &o.OrderID, &o.RestaurantID, &o.CustomerID,
			&o.PickupLatitude, &o.PickupLongitude, &o.PickupAddress,
			&o.DropLatitude, &o.DropLongitude, &o.DropAddress,
			&o.Amount, &o.PaymentMode, &o.DeliveryStatus, &o.AssignedRiderID,
			&o.CreatedAt, &o.UpdatedAt, &o.AssignedAt, &o.PickedUpAt, &o.DeliveredAt,
			&o.RiderUserID, &o.AssignmentType, &o.RestaurantOwned, &o.RestaurantName, &o.RestaurantPhone,
			&o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.RiderArrivedAt); err != nil {
			return nil, err
		}
		orders = append(orders, &o)
	}
	return orders, nil
}

func (r *DeliveryRepository) GetActiveOrderForRider(ctx context.Context, riderUserID string) (*models.DeliveryOrder, error) {
	var o models.DeliveryOrder
	err := r.db.QueryRowContext(ctx,
		`SELECT delivery_order_id, order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address, drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, created_at, updated_at, assigned_at, picked_up_at, delivered_at, rider_user_id, assignment_type, restaurant_owned, restaurant_name, restaurant_phone, customer_name, customer_phone, items_summary, rider_arrived_at
		 FROM delivery_orders
		 WHERE (rider_user_id=$1 OR assigned_rider_id=$1)
		 AND delivery_status IN ('rider_assigned', 'rider_arrived_restaurant', 'picked_up', 'on_the_way')
		 ORDER BY COALESCE(assigned_at, updated_at, created_at) DESC
		 LIMIT 1`, riderUserID,
	).Scan(&o.DeliveryOrderID, &o.OrderID, &o.RestaurantID, &o.CustomerID,
		&o.PickupLatitude, &o.PickupLongitude, &o.PickupAddress,
		&o.DropLatitude, &o.DropLongitude, &o.DropAddress,
		&o.Amount, &o.PaymentMode, &o.DeliveryStatus, &o.AssignedRiderID,
		&o.CreatedAt, &o.UpdatedAt, &o.AssignedAt, &o.PickedUpAt, &o.DeliveredAt,
		&o.RiderUserID, &o.AssignmentType, &o.RestaurantOwned, &o.RestaurantName, &o.RestaurantPhone,
		&o.CustomerName, &o.CustomerPhone, &o.ItemsSummary, &o.RiderArrivedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *DeliveryRepository) GetRestaurantContact(ctx context.Context, restaurantID int) (string, string, error) {
	var name, phone string
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(r.name, ''), COALESCE(u.business_phone, u.phone, '')
		 FROM restaurants r
		 LEFT JOIN users u ON u.id = r.user_id
		 WHERE r.restaurant_id=$1`, restaurantID,
	).Scan(&name, &phone)
	return name, phone, err
}

func (r *DeliveryRepository) UpdateDeliveryStatus(ctx context.Context, tx *sql.Tx, deliveryOrderID int, status string) error {
	q := `UPDATE delivery_orders SET delivery_status=$2, updated_at=NOW() WHERE delivery_order_id=$1`
	if tx != nil {
		_, err := tx.ExecContext(ctx, q, deliveryOrderID, status)
		return err
	}
	_, err := r.db.ExecContext(ctx, q, deliveryOrderID, status)
	return err
}

func (r *DeliveryRepository) AssignRider(ctx context.Context, tx *sql.Tx, deliveryOrderID int, riderID string) error {
	q := `UPDATE delivery_orders SET assigned_rider_id=$2, rider_user_id=$2, delivery_status='rider_assigned', assigned_at=NOW(), updated_at=NOW()
	      WHERE delivery_order_id=$1 AND assigned_rider_id IS NULL`
	var result sql.Result
	var err error
	if tx != nil {
		result, err = tx.ExecContext(ctx, q, deliveryOrderID, riderID)
	} else {
		result, err = r.db.ExecContext(ctx, q, deliveryOrderID, riderID)
	}
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("delivery order already assigned")
	}
	return nil
}

func (r *DeliveryRepository) UpdateDeliveryTimestamp(ctx context.Context, tx *sql.Tx, deliveryOrderID int, status string) error {
	var q string
	switch status {
	case models.DeliveryStatusRiderArrivedRestaurant:
		q = `UPDATE delivery_orders SET rider_arrived_at=NOW(), delivery_status=$2, updated_at=NOW() WHERE delivery_order_id=$1`
	case models.DeliveryStatusPickedUp:
		q = `UPDATE delivery_orders SET picked_up_at=NOW(), delivery_status=$2, updated_at=NOW() WHERE delivery_order_id=$1`
	case models.DeliveryStatusDelivered:
		q = `UPDATE delivery_orders SET delivered_at=NOW(), delivery_status=$2, updated_at=NOW() WHERE delivery_order_id=$1`
	default:
		q = `UPDATE delivery_orders SET delivery_status=$2, updated_at=NOW() WHERE delivery_order_id=$1`
	}
	if tx != nil {
		_, err := tx.ExecContext(ctx, q, deliveryOrderID, status)
		return err
	}
	_, err := r.db.ExecContext(ctx, q, deliveryOrderID, status)
	return err
}

func (r *DeliveryRepository) RecordStatusHistory(ctx context.Context, tx *sql.Tx, orderID int, fromStatus, toStatus string, changedBy string) error {
	query := `INSERT INTO delivery_status_history (order_id, from_status, to_status, changed_by)
		VALUES ($1, $2, $3, $4)`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, orderID, fromStatus, toStatus, changedBy)
	} else {
		_, err = r.db.ExecContext(ctx, query, orderID, fromStatus, toStatus, changedBy)
	}
	return err
}

func (r *DeliveryRepository) RecordEarning(ctx context.Context, tx *sql.Tx, riderID string, orderID int, earningType string, amount float64, description string) error {
	query := `INSERT INTO rider_earnings (rider_id, order_id, type, amount, description) VALUES ($1, $2, $3, $4, $5)`
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, riderID, orderID, earningType, amount, description)
	} else {
		_, err = r.db.ExecContext(ctx, query, riderID, orderID, earningType, amount, description)
	}
	return err
}

// ===== DELIVERY ORDER REQUESTS =====

func (r *DeliveryRepository) CreateRequest(ctx context.Context, deliveryOrderID, orderID int, riderID string, distanceKm float64, expiresAt time.Time) (*models.DeliveryOrderRequest, error) {
	var req models.DeliveryOrderRequest
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO delivery_order_requests (delivery_order_id, order_id, rider_id, distance_km, expires_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (delivery_order_id, rider_id) DO UPDATE SET
			order_id = EXCLUDED.order_id,
			distance_km = EXCLUDED.distance_km,
			status = 'pending',
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
		 WHERE delivery_order_requests.status IN ('rejected', 'expired', 'cancelled')
		 RETURNING request_id, delivery_order_id, order_id, rider_id, status, distance_km, expires_at, created_at, updated_at`,
		deliveryOrderID, orderID, riderID, distanceKm, expiresAt,
	).Scan(&req.RequestID, &req.DeliveryOrderID, &req.OrderID, &req.RiderID,
		&req.Status, &req.DistanceKm, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *DeliveryRepository) GetPendingRequestsForRider(ctx context.Context, riderID string) ([]*models.DeliveryOrderRequest, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT request_id, delivery_order_id, order_id, rider_id, status, distance_km, expires_at, created_at, updated_at
		 FROM delivery_order_requests
		 WHERE rider_id=$1 AND status='pending' AND expires_at > NOW()
		 ORDER BY created_at DESC`, riderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

func (r *DeliveryRepository) GetRequestByID(ctx context.Context, requestID int) (*models.DeliveryOrderRequest, error) {
	var req models.DeliveryOrderRequest
	err := r.db.QueryRowContext(ctx,
		`SELECT request_id, delivery_order_id, order_id, rider_id, status, distance_km, expires_at, created_at, updated_at
		 FROM delivery_order_requests WHERE request_id=$1`, requestID,
	).Scan(&req.RequestID, &req.DeliveryOrderID, &req.OrderID, &req.RiderID,
		&req.Status, &req.DistanceKm, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *DeliveryRepository) GetRequestByIDForUpdate(ctx context.Context, tx *sql.Tx, requestID int) (*models.DeliveryOrderRequest, error) {
	var req models.DeliveryOrderRequest
	err := tx.QueryRowContext(ctx,
		`SELECT request_id, delivery_order_id, order_id, rider_id, status, distance_km, expires_at, created_at, updated_at
		 FROM delivery_order_requests WHERE request_id=$1 FOR UPDATE`, requestID,
	).Scan(&req.RequestID, &req.DeliveryOrderID, &req.OrderID, &req.RiderID,
		&req.Status, &req.DistanceKm, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *DeliveryRepository) AcceptRequest(ctx context.Context, tx *sql.Tx, requestID int) error {
	q := `UPDATE delivery_order_requests SET status='accepted', updated_at=NOW() WHERE request_id=$1 AND status='pending'`
	result, err := tx.ExecContext(ctx, q, requestID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("request not pending or already responded")
	}
	return nil
}

func (r *DeliveryRepository) RejectRequest(ctx context.Context, requestID int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE delivery_order_requests SET status='rejected', updated_at=NOW() WHERE request_id=$1 AND status='pending'`,
		requestID)
	return err
}

func (r *DeliveryRepository) CancelOtherRequests(ctx context.Context, tx *sql.Tx, deliveryOrderID int, exceptRequestID int) ([]string, error) {
	q := `UPDATE delivery_order_requests SET status='cancelled', updated_at=NOW()
	      WHERE delivery_order_id=$1 AND request_id!=$2 AND status='pending'
	      RETURNING rider_id`
	rows, err := tx.QueryContext(ctx, q, deliveryOrderID, exceptRequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var riderIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		riderIDs = append(riderIDs, id)
	}
	return riderIDs, nil
}

func (r *DeliveryRepository) ExpirePendingRequests(ctx context.Context) ([]models.DeliveryOrderRequest, error) {
	rows, err := r.db.QueryContext(ctx,
		`UPDATE delivery_order_requests SET status='expired', updated_at=NOW()
		 WHERE status='pending' AND expires_at <= NOW()
		 RETURNING request_id, delivery_order_id, order_id, rider_id, status, distance_km, expires_at, created_at, updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var expired []models.DeliveryOrderRequest
	for rows.Next() {
		var req models.DeliveryOrderRequest
		if err := rows.Scan(&req.RequestID, &req.DeliveryOrderID, &req.OrderID, &req.RiderID,
			&req.Status, &req.DistanceKm, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt); err != nil {
			return nil, err
		}
		expired = append(expired, req)
	}
	return expired, nil
}

func (r *DeliveryRepository) CountPendingForOrder(ctx context.Context, deliveryOrderID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM delivery_order_requests WHERE delivery_order_id=$1 AND status='pending'`,
		deliveryOrderID).Scan(&count)
	return count, err
}

func (r *DeliveryRepository) HasAcceptedRequest(ctx context.Context, deliveryOrderID int) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM delivery_order_requests WHERE delivery_order_id=$1 AND status='accepted')`,
		deliveryOrderID).Scan(&exists)
	return exists, err
}

// ===== RIDER AVAILABILITY =====

func (r *DeliveryRepository) UpsertRiderAvailability(ctx context.Context, riderID string, isOnline, isAvailable bool, currentOrderID *int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO rider_availability (rider_id, is_online, is_available, current_order_id, updated_at)
		 VALUES ($1,$2,$3,$4,NOW())
		 ON CONFLICT (rider_id) DO UPDATE SET is_online=$2, is_available=$3, current_order_id=$4, updated_at=NOW()`,
		riderID, isOnline, isAvailable, currentOrderID)
	return err
}

func (r *DeliveryRepository) SetRiderBusy(ctx context.Context, tx *sql.Tx, riderID string, orderID int) error {
	q := `INSERT INTO rider_availability (rider_id, is_online, is_available, current_order_id, updated_at)
	      VALUES ($1, true, false, $2, NOW())
	      ON CONFLICT (rider_id) DO UPDATE SET is_available=false, current_order_id=$2, updated_at=NOW()`
	if tx != nil {
		_, err := tx.ExecContext(ctx, q, riderID, orderID)
		return err
	}
	_, err := r.db.ExecContext(ctx, q, riderID, orderID)
	return err
}

func (r *DeliveryRepository) SetRiderFree(ctx context.Context, tx *sql.Tx, riderID string) error {
	q := `UPDATE rider_availability SET is_available=true, current_order_id=NULL, updated_at=NOW() WHERE rider_id=$1`
	if tx != nil {
		_, err := tx.ExecContext(ctx, q, riderID)
		return err
	}
	_, err := r.db.ExecContext(ctx, q, riderID)
	return err
}

func (r *DeliveryRepository) GetRiderAvailability(ctx context.Context, riderID string) (*models.RiderAvailability, error) {
	var a models.RiderAvailability
	err := r.db.QueryRowContext(ctx,
		`SELECT rider_id, is_online, is_available, current_order_id, updated_at FROM rider_availability WHERE rider_id=$1`,
		riderID).Scan(&a.RiderID, &a.IsOnline, &a.IsAvailable, &a.CurrentOrderID, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ===== RIDER LOCATIONS =====

func (r *DeliveryRepository) UpsertRiderLocation(ctx context.Context, riderID string, lat, lng float64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO rider_locations (rider_id, latitude, longitude, last_updated_at)
		 VALUES ($1,$2,$3,NOW())
		 ON CONFLICT (rider_id) DO UPDATE SET latitude=$2, longitude=$3, last_updated_at=NOW()`,
		riderID, lat, lng)
	return err
}

func (r *DeliveryRepository) GetRiderLocation(ctx context.Context, riderID string) (*models.RiderLocation, error) {
	var loc models.RiderLocation
	err := r.db.QueryRowContext(ctx,
		`SELECT rider_id, latitude, longitude, last_updated_at FROM rider_locations WHERE rider_id=$1`,
		riderID).Scan(&loc.RiderID, &loc.Latitude, &loc.Longitude, &loc.LastUpdatedAt)
	if err != nil {
		return nil, err
	}
	return &loc, nil
}

// ===== NEAREST RIDER SEARCH (Haversine) =====

func (r *DeliveryRepository) FindNearestRiders(ctx context.Context, pickupLat, pickupLng, radiusKm float64, maxRiders int) ([]models.NearbyRider, error) {
	// Haversine formula in SQL. Riders must be online, available, no current order, location updated within 5 min.
	query := `
		SELECT rl.rider_id, rl.latitude, rl.longitude,
			(6371 * acos(
				LEAST(1.0, cos(radians($1)) * cos(radians(rl.latitude)) * cos(radians(rl.longitude) - radians($2))
				+ sin(radians($1)) * sin(radians(rl.latitude)))
			)) AS distance_km
		FROM rider_locations rl
		INNER JOIN rider_availability ra ON ra.rider_id = rl.rider_id
		WHERE ra.is_online = true
		  AND ra.is_available = true
		  AND ra.current_order_id IS NULL
		  AND rl.last_updated_at >= NOW() - INTERVAL '5 minutes'
		HAVING (6371 * acos(
				LEAST(1.0, cos(radians($1)) * cos(radians(rl.latitude)) * cos(radians(rl.longitude) - radians($2))
				+ sin(radians($1)) * sin(radians(rl.latitude)))
			)) <= $3
		ORDER BY distance_km ASC
		LIMIT $4`

	rows, err := r.db.QueryContext(ctx, query, pickupLat, pickupLng, radiusKm, maxRiders)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var riders []models.NearbyRider
	for rows.Next() {
		var nr models.NearbyRider
		if err := rows.Scan(&nr.RiderID, &nr.Latitude, &nr.Longitude, &nr.DistanceKm); err != nil {
			return nil, err
		}
		nr.DistanceKm = math.Round(nr.DistanceKm*100) / 100
		riders = append(riders, nr)
	}
	return riders, nil
}

func (r *DeliveryRepository) GetRiderEligibilitySummary(
	ctx context.Context,
	pickupLat, pickupLng, radiusKm float64,
) (*RiderEligibilitySummary, error) {
	const query = `
		WITH rider_candidates AS (
			SELECT
				ra.is_online,
				ra.is_available,
				ra.current_order_id,
				rl.rider_id IS NOT NULL AS has_location,
				COALESCE(rl.last_updated_at >= NOW() - INTERVAL '5 minutes', false) AS has_fresh_location,
				CASE
					WHEN rl.rider_id IS NULL THEN NULL
					ELSE (6371 * acos(
						LEAST(1.0, cos(radians($1)) * cos(radians(rl.latitude))
						* cos(radians(rl.longitude) - radians($2))
						+ sin(radians($1)) * sin(radians(rl.latitude)))
					))
				END AS distance_km
			FROM rider_availability ra
			LEFT JOIN rider_locations rl ON rl.rider_id = ra.rider_id
		)
		SELECT
			COUNT(*) FILTER (WHERE is_online),
			COUNT(*) FILTER (
				WHERE is_online AND is_available AND current_order_id IS NULL
			),
			COUNT(*) FILTER (
				WHERE is_online AND is_available AND current_order_id IS NULL AND has_location
			),
			COUNT(*) FILTER (
				WHERE is_online AND is_available AND current_order_id IS NULL
				  AND has_location AND has_fresh_location
			),
			COUNT(*) FILTER (
				WHERE is_online AND is_available AND current_order_id IS NULL
				  AND has_location AND has_fresh_location AND distance_km <= $3
			)
		FROM rider_candidates`

	var summary RiderEligibilitySummary
	err := r.db.QueryRowContext(ctx, query, pickupLat, pickupLng, radiusKm).Scan(
		&summary.OnlineRiders,
		&summary.AvailableRiders,
		&summary.RidersWithLocation,
		&summary.RidersWithFreshGPS,
		&summary.RidersWithinRadius,
	)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// BeginTx starts a transaction.
func (r *DeliveryRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

func scanRequests(rows *sql.Rows) ([]*models.DeliveryOrderRequest, error) {
	var reqs []*models.DeliveryOrderRequest
	for rows.Next() {
		var req models.DeliveryOrderRequest
		if err := rows.Scan(&req.RequestID, &req.DeliveryOrderID, &req.OrderID, &req.RiderID,
			&req.Status, &req.DistanceKm, &req.ExpiresAt, &req.CreatedAt, &req.UpdatedAt); err != nil {
			return nil, err
		}
		reqs = append(reqs, &req)
	}
	return reqs, nil
}

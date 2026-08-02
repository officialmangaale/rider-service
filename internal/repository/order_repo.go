package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// OrderRepository provides data access for the shared orders table.
type OrderRepository struct {
	db *sql.DB
}

// NewOrderRepository creates a new OrderRepository.
func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

const orderSelectColumns = `
	o.order_id, o.customer_id, o.restaurant_id, o.delivery_partner_id,
	o.order_status, o.payment_status,
	o.subtotal, o.tax_amount, o.delivery_fee, o.tip_amount, o.discount_amount, o.total_amount,
	o.delivery_address, o.delivery_latitude, o.delivery_longitude,
	o.order_type, o.order_number,
	o.estimated_delivery_time, o.actual_delivery_time,
	o.created_at, o.updated_at,
	COALESCE(r.name, '') as restaurant_name,
	COALESCE(r.street_address, '') as restaurant_address,
	r.latitude as restaurant_lat,
	r.longitude as restaurant_lng,
	COALESCE(ru.business_phone, ru.phone, '') as restaurant_phone,
	'' as customer_phone
`

const orderFromJoin = `
	FROM orders o
	LEFT JOIN restaurants r ON r.restaurant_id = o.restaurant_id
	LEFT JOIN users ru ON ru.id = r.user_id
`

const activeOrderSelectColumns = `
	o.order_id, o.customer_id, o.restaurant_id, COALESCE(o.delivery_partner_id, o.assigned_rider_user_id) as delivery_partner_id,
	COALESCE(NULLIF(o.delivery_status, ''), o.order_status::text) as order_status, o.payment_status,
	o.subtotal, o.tax_amount, o.delivery_fee, o.tip_amount, o.discount_amount, o.total_amount,
	o.delivery_address, o.delivery_latitude, o.delivery_longitude,
	o.order_type, o.order_number,
	o.estimated_delivery_time, o.actual_delivery_time,
	o.created_at, o.updated_at,
	COALESCE(r.name, '') as restaurant_name,
	COALESCE(r.street_address, '') as restaurant_address,
	r.latitude as restaurant_lat,
	r.longitude as restaurant_lng,
	COALESCE(ru.business_phone, ru.phone, '') as restaurant_phone,
	'' as customer_phone
`

func scanOrder(row interface{ Scan(...interface{}) error }) (*models.Order, error) {
	var o models.Order
	err := row.Scan(
		&o.OrderID, &o.CustomerID, &o.RestaurantID, &o.DeliveryPartnerID,
		&o.OrderStatus, &o.PaymentStatus,
		&o.Subtotal, &o.TaxAmount, &o.DeliveryFee, &o.TipAmount, &o.DiscountAmount, &o.TotalAmount,
		&o.DeliveryAddress, &o.DeliveryLatitude, &o.DeliveryLongitude,
		&o.OrderType, &o.OrderNumber,
		&o.EstimatedDelivery, &o.ActualDelivery,
		&o.CreatedAt, &o.UpdatedAt,
		&o.RestaurantName, &o.RestaurantAddress,
		&o.RestaurantLat, &o.RestaurantLng,
		&o.RestaurantPhone, &o.CustomerPhone,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetActiveOrderForRider returns the rider's current active delivery order.
func (r *OrderRepository) GetActiveOrderForRider(ctx context.Context, riderID string) (*models.Order, error) {
	query := fmt.Sprintf(`SELECT %s %s
		WHERE (
			o.delivery_partner_id = $1
			AND o.order_status IN ('ready', 'out_for_delivery')
		) OR (
			o.assigned_rider_user_id = $1
			AND o.delivery_status IN ('rider_assigned', 'rider_arrived_restaurant', 'picked_up', 'on_the_way')
		)
		ORDER BY COALESCE(o.assigned_at, o.updated_at) DESC LIMIT 1`, activeOrderSelectColumns, orderFromJoin)
	row := r.db.QueryRowContext(ctx, query, riderID)
	return scanOrder(row)
}

// GetOrderByID returns an order by ID with restaurant details joined.
func (r *OrderRepository) GetOrderByID(ctx context.Context, orderID int) (*models.Order, error) {
	query := fmt.Sprintf(`SELECT %s %s WHERE o.order_id = $1`, orderSelectColumns, orderFromJoin)
	row := r.db.QueryRowContext(ctx, query, orderID)
	return scanOrder(row)
}

// GetAvailableOrders returns ready orders without a delivery partner, near the rider's location.
func (r *OrderRepository) GetAvailableOrders(ctx context.Context, limit, offset int) ([]*models.Order, int64, error) {
	countQuery := `SELECT COUNT(*) FROM orders WHERE order_status = 'ready' AND delivery_partner_id IS NULL AND order_type = 'DELIVERY'`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s %s
		WHERE o.order_status = 'ready'
		AND o.delivery_partner_id IS NULL
		AND o.order_type = 'DELIVERY'
		ORDER BY o.created_at ASC
		LIMIT $1 OFFSET $2`, orderSelectColumns, orderFromJoin)

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	return orders, total, nil
}

// GetOrderHistoryForRider returns past delivered/cancelled/failed orders for a rider.
func (r *OrderRepository) GetOrderHistoryForRider(ctx context.Context, riderID string, limit, offset int) ([]*models.Order, int64, error) {
	countQuery := `SELECT COUNT(*) FROM orders WHERE delivery_partner_id = $1 AND order_status IN ('delivered', 'cancelled', 'failed')`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, riderID).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`SELECT %s %s
		WHERE o.delivery_partner_id = $1
		AND o.order_status IN ('delivered', 'cancelled', 'failed')
		ORDER BY o.updated_at DESC
		LIMIT $2 OFFSET $3`, orderSelectColumns, orderFromJoin)

	rows, err := r.db.QueryContext(ctx, query, riderID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, o)
	}
	return orders, total, nil
}

// BeginTx starts a database transaction.
func (r *OrderRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

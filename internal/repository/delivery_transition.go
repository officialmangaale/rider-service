package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
)

// AdvanceDelivery commits projection, history, workload release and the existing
// earning together. Concurrent/retried transitions cannot duplicate a payout or
// overwrite a withdrawal. Canonical kitchen transitions still run in restaurant-service.
func (r *DeliveryRepository) AdvanceDelivery(ctx context.Context, order *models.DeliveryOrder, riderID, next string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if !order.IsGrocery() {
		if err := r.LockFoodOrder(ctx, tx, order.OrderID); err != nil {
			return false, err
		}
		var allowed bool
		err = tx.QueryRowContext(ctx, `SELECT COALESCE((COALESCE(assigned_rider_user_id,'')=$2 OR COALESCE(delivery_partner_id,'')=$2 OR rider_id::text=$2)
		    AND (($3='rider_arrived_restaurant' AND order_status IN ('preparing','ready'))
		      OR ($3 IN ('picked_up','on_the_way') AND order_status='out_for_delivery')
		      OR ($3='delivered' AND (order_status='delivered' OR (order_status='completed' AND delivered_at IS NOT NULL)))),false)
		    FROM orders WHERE order_id=$1`, order.OrderID, riderID, next).Scan(&allowed)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, fmt.Errorf("order is no longer assigned or active")
		}
	}
	var current, assigned string
	err = tx.QueryRowContext(ctx, `SELECT delivery_status,COALESCE(assigned_rider_id,rider_user_id,'')
        FROM delivery_orders WHERE delivery_order_id=$1 FOR UPDATE`, order.DeliveryOrderID).Scan(&current, &assigned)
	if err != nil {
		return false, err
	}
	if assigned != riderID {
		return false, fmt.Errorf("order is no longer assigned to this rider")
	}
	if current == next {
		return false, nil
	}
	if current != order.DeliveryStatus {
		return false, fmt.Errorf("delivery state changed; refresh the order")
	}
	if err = r.UpdateDeliveryTimestamp(ctx, tx, order.DeliveryOrderID, next); err != nil {
		return false, err
	}
	if err = r.RecordDeliveryStatusHistory(ctx, tx, order.OrderType, order.OrderID, current, next, riderID); err != nil {
		return false, err
	}
	if !order.IsGrocery() {
		if _, err = tx.ExecContext(ctx, `UPDATE orders SET delivery_status=$2,updated_at=NOW() WHERE order_id=$1`, order.OrderID, next); err != nil {
			return false, err
		}
	}
	if next == models.DeliveryStatusDelivered {
		if _, err = tx.ExecContext(ctx, `UPDATE rider_availability SET current_order_id=NULL,is_available=is_online,updated_at=NOW()
            WHERE rider_id=$1 AND current_order_id=$2`, riderID, order.OrderID); err != nil {
			return false, err
		}
		if !order.RestaurantOwned {
			// Payout comes from the order's immutable pricing snapshot when one
			// exists (single source of truth); otherwise the legacy flat payout.
			payout, perr := r.riderPayoutFor(ctx, tx, order)
			if perr != nil {
				return false, perr
			}
			if err = r.RecordEarning(ctx, tx, riderID, order.OrderID, "delivery_fee", payout, "Base delivery payout", order.OrderType); err != nil {
				return false, err
			}
			// Platform upgrade Module 11: mirror the same earning into the
			// rider's net-payable wallet ledger — a separate concern from
			// rider_earnings (gross earnings history). The wallet balance is
			// what the platform currently owes the rider net of any COD cash
			// the rider still holds, which rider_earnings alone can't express.
			// Gated on !RestaurantOwned exactly like the payout above: a
			// restaurant-owned rider's arrangement (earnings and any COD
			// they collect) is with that restaurant, not the platform.
			orderID := order.OrderID
			walletOrderType := "food"
			if order.IsGrocery() {
				walletOrderType = "grocery"
			}
			if _, err = postWalletTransaction(ctx, tx, riderID, models.WalletTxnDeliveryEarning, payout,
				&orderID, nil, nil, nil, nil, walletOrderType); err != nil {
				return false, err
			}
			if isCODPaymentMode(order.PaymentMode) && order.Amount > 0 {
				note := "COD cash collected on delivery"
				if _, err = postWalletTransaction(ctx, tx, riderID, models.WalletTxnCashCollected, -order.Amount,
					&orderID, nil, nil, &note, nil, walletOrderType); err != nil {
					return false, err
				}
			}
		}
	}
	return true, tx.Commit()
}

// legacyFlatRiderPayout is the payout used before per-order pricing snapshots
// existed; it equals the pricing engine's default rider cost.
const legacyFlatRiderPayout = 30.00

// riderPayoutFor returns the rider's payout for a completed delivery: the
// order's pricing-snapshot rider cost when a snapshot exists, else the legacy
// flat amount. Grocery and food read separate snapshots, so overlapping order
// IDs cannot substitute another business's payout.
func (r *DeliveryRepository) riderPayoutFor(ctx context.Context, tx *sql.Tx, order *models.DeliveryOrder) (float64, error) {
	return riderPayoutFromSnapshot(ctx, tx, order)
}

// QuotedRiderPayout uses the same immutable source as completion, including a
// configured zero payout. Customer delivery charges are never rider earnings.
func (r *DeliveryRepository) QuotedRiderPayout(ctx context.Context, order *models.DeliveryOrder) (float64, error) {
	return riderPayoutFromSnapshot(ctx, r.db, order)
}

func riderPayoutFromSnapshot(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, order *models.DeliveryOrder) (float64, error) {
	table := "order_pricing_snapshots"
	query := `SELECT rider_cost_cents FROM order_pricing_snapshots WHERE order_id=$1`
	if order.IsGrocery() {
		table = "grocery_pricing_snapshots"
		query = `SELECT rider_cost_cents FROM grocery_pricing_snapshots WHERE grocery_order_id=$1`
	}
	var hasTable bool
	if err := q.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&hasTable); err != nil {
		return 0, err
	}
	if !hasTable {
		return legacyFlatRiderPayout, nil
	}
	var cents sql.NullInt64
	err := q.QueryRowContext(ctx, query, order.OrderID).Scan(&cents)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !cents.Valid) {
		return legacyFlatRiderPayout, nil
	}
	if err != nil {
		return 0, err
	}
	return float64(cents.Int64) / 100, nil
}

// isCODPaymentMode mirrors the check already used at the order-status-update
// gate (internal/service/delivery_service.go) that requires cash-collection
// confirmation before a COD order can reach "delivered".
func isCODPaymentMode(paymentMode string) bool {
	return paymentMode == "cod" || paymentMode == "cash"
}

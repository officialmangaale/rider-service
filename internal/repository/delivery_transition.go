package repository

import (
	"context"
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
	if err = r.RecordStatusHistory(ctx, tx, order.OrderID, current, next, riderID); err != nil {
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
			// Existing payout policy; this change does not introduce a price.
			if err = r.RecordEarning(ctx, tx, riderID, order.OrderID, "delivery_fee", 30.00, "Base delivery payout", order.OrderType); err != nil {
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
			if _, err = PostWalletTransaction(ctx, tx, riderID, models.WalletTxnDeliveryEarning, 30.00,
				&orderID, nil, nil, nil, nil); err != nil {
				return false, err
			}
			if isCODPaymentMode(order.PaymentMode) && order.Amount > 0 {
				note := "COD cash collected on delivery"
				if _, err = PostWalletTransaction(ctx, tx, riderID, models.WalletTxnCashCollected, -order.Amount,
					&orderID, nil, nil, &note, nil); err != nil {
					return false, err
				}
			}
		}
	}
	return true, tx.Commit()
}

// isCODPaymentMode mirrors the check already used at the order-status-update
// gate (internal/service/delivery_service.go) that requires cash-collection
// confirmation before a COD order can reach "delivered".
func isCODPaymentMode(paymentMode string) bool {
	return paymentMode == "cod" || paymentMode == "cash"
}

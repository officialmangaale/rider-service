package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/riderpay"
	"math"
)

func (r *DeliveryRepository) ConfigureContractPay(ctx context.Context, enabled bool) error {
	var ready bool
	if err := r.db.QueryRowContext(ctx, `SELECT to_regclass('rider_offer_pay') IS NOT NULL AND to_regclass('rider_assignment_pay') IS NOT NULL`).Scan(&ready); err != nil {
		return err
	}
	if enabled && !ready {
		return errors.New("rider contract pay requires migration 147")
	}
	r.ContractPayEnabled = enabled
	r.ContractSnapshotsAvailable = ready
	return nil
}

func (r *DeliveryRepository) freezeOfferPay(ctx context.Context, tx *sql.Tx, req *models.DeliveryOrderRequest) error {
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT to_jsonb(c) FROM rider_earning_contracts c WHERE rider_id=$1 AND enabled FOR SHARE`, req.RiderID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var policy riderpay.Policy
	if err = json.Unmarshal(raw, &policy); err != nil {
		return err
	}
	var breakdown []byte
	var kind string
	var owned bool
	err = tx.QueryRowContext(ctx, `SELECT d.order_type,COALESCE(d.restaurant_owned,false),s.breakdown FROM delivery_orders d LEFT JOIN order_pricing_snapshots s ON s.order_id=d.order_id AND d.order_type='food' WHERE d.delivery_order_id=$1`, req.DeliveryOrderID).Scan(&kind, &owned, &breakdown)
	if err != nil {
		return err
	}
	if kind != "food" || owned {
		return nil
	}
	var b struct {
		Items    float64 `json:"customer_item_total"`
		Coupon   float64 `json:"coupon_discount"`
		Referral float64 `json:"referral_discount"`
		Km       float64 `json:"delivery_distance_km"`
		Tip      float64 `json:"tip_amount"`
	}
	if err = json.Unmarshal(breakdown, &b); err != nil {
		return err
	}
	pay, incentive, err := policy.Quote(int64(math.Round(math.Max(0, b.Items-b.Coupon-b.Referral)*100)), b.Km)
	if err != nil {
		return err
	}
	tip := int64(math.Round(b.Tip * 100))
	if tip < 0 {
		return errors.New("invalid tip")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO rider_offer_pay(request_id,rider_id,delivery_order_id,amount_cents,incentive_cents,tip_cents,policy)VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(request_id)DO NOTHING`, req.RequestID, req.RiderID, req.DeliveryOrderID, pay+tip, incentive, tip, string(raw))
	return err
}

func (r *DeliveryRepository) RequestPay(ctx context.Context, req *models.DeliveryOrderRequest) (*float64, error) {
	if !r.ContractPayEnabled && !r.ContractSnapshotsAvailable {
		return nil, nil
	}
	var cents int64
	var mode string
	err := r.db.QueryRowContext(ctx, `SELECT amount_cents,policy->>'mode' FROM rider_offer_pay WHERE request_id=$1 AND rider_id=$2`, req.RequestID, req.RiderID).Scan(&cents, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := float64(cents) / 100
	req.EarningMode = mode
	return &value, nil
}

func (r *DeliveryRepository) assignmentPay(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, order *models.DeliveryOrder) (*float64, error) {
	if (!r.ContractPayEnabled && !r.ContractSnapshotsAvailable) || order.RestaurantOwned || order.IsGrocery() {
		return nil, nil
	}
	rider := ""
	if order.AssignedRiderID != nil {
		rider = *order.AssignedRiderID
	}
	if rider == "" && order.RiderUserID != nil {
		rider = *order.RiderUserID
	}
	if rider == "" {
		return nil, nil
	}
	var cents int64
	err := q.QueryRowContext(ctx, `SELECT amount_cents FROM rider_assignment_pay WHERE delivery_order_id=$1 AND rider_id=$2`, order.DeliveryOrderID, rider).Scan(&cents)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := float64(cents) / 100
	return &value, nil
}

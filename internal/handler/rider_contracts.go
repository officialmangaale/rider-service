package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/riderpay"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func (h *AdminRiderHandler) GetEarningContract(c *gin.Context) {
	var raw []byte
	err := h.db.QueryRowContext(c.Request.Context(), `SELECT to_jsonb(p) FROM rider_earning_contracts p WHERE rider_id=$1`, c.Param("riderId")).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		dto.Success(c, 200, "No per-rider contract configured", gin.H{"contract": nil, "rollout_enabled": h.ContractPayEnabled})
		return
	}
	if err != nil {
		dto.InternalError(c, "Contract unavailable")
		return
	}
	dto.Success(c, 200, "Rider earning contract", gin.H{"contract": json.RawMessage(raw), "rollout_enabled": h.ContractPayEnabled})
}
func (h *AdminRiderHandler) SaveEarningContract(c *gin.Context) {
	var p riderpay.Policy
	if c.ShouldBindJSON(&p) != nil {
		dto.ValidationError(c, "Invalid earning contract")
		return
	}
	p.RiderID = c.Param("riderId")
	if err := p.Validate(); err != nil {
		dto.ValidationError(c, err.Error())
		return
	}
	cancellations, _ := json.Marshal(p.Cancellation)
	if string(cancellations) == "null" {
		cancellations = []byte(`{}`)
	}
	var version int64
	args := []any{p.RiderID, p.Enabled, p.Mode, p.BaseCents, p.PerKmCents, p.SalaryCents, p.SalaryPeriod, p.IncentiveCents, p.IncentivePermille, string(cancellations), middleware.GetUserID(c), p.Version}
	query := `INSERT INTO rider_earning_contracts(rider_id,enabled,mode,base_cents,per_km_cents,salary_cents,salary_period,incentive_cents,incentive_permille,cancellation,updated_by)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11 WHERE $12::bigint=0 ON CONFLICT DO NOTHING RETURNING version`
	if p.Version > 0 {
		query = `UPDATE rider_earning_contracts SET enabled=$2,mode=$3,base_cents=$4,per_km_cents=$5,salary_cents=$6,salary_period=$7,incentive_cents=$8,incentive_permille=$9,cancellation=$10::jsonb,updated_by=$11 WHERE rider_id=$1 AND version=$12 RETURNING version`
	}
	err := h.db.QueryRowContext(c.Request.Context(), query, args...).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		dto.Conflict(c, "Contract changed; refresh before saving")
		return
	}
	if err != nil {
		dto.InternalError(c, "Contract could not be saved")
		return
	}
	h.GetEarningContract(c)
}
func (h *AdminRiderHandler) CancellationPayReviews(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT r.id,r.rider_id,r.delivery_order_id,d.order_id,r.stage,r.proposed_cents,r.status FROM rider_cancellation_pay_reviews r JOIN delivery_orders d USING(delivery_order_id) ORDER BY (r.status='pending') DESC,r.created_at DESC LIMIT 200`)
	if err != nil {
		dto.InternalError(c, "Cancellation reviews unavailable")
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, delivery, order, cents int64
		var rider, stage, status string
		if rows.Scan(&id, &rider, &delivery, &order, &stage, &cents, &status) != nil {
			dto.InternalError(c, "Cancellation reviews unavailable")
			return
		}
		out = append(out, gin.H{"id": id, "rider_id": rider, "delivery_order_id": delivery, "order_id": order, "stage": stage, "proposed_cents": cents, "status": status})
	}
	if rows.Err() != nil {
		dto.InternalError(c, "Cancellation reviews unavailable")
		return
	}
	dto.Success(c, 200, "Cancellation pay reviews", gin.H{"items": out, "rollout_enabled": h.ContractPayEnabled})
}
func (h *AdminRiderHandler) ReviewCancellationPay(c *gin.Context) {
	if !h.ContractPayEnabled {
		dto.Conflict(c, "Rider contract pay rollout is inactive")
		return
	}
	id, err := strconv.ParseInt(c.Param("reviewId"), 10, 64)
	var req struct {
		Approve *bool  `json:"approve"`
		Note    string `json:"note"`
	}
	if err != nil || id <= 0 || c.ShouldBindJSON(&req) != nil || req.Approve == nil || strings.TrimSpace(req.Note) == "" || len(req.Note) > 1000 {
		dto.ValidationError(c, "Decision and eligibility review note required")
		return
	}
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		dto.InternalError(c, "Review unavailable")
		return
	}
	defer tx.Rollback()
	var rider, status, deliveryStatus string
	var cents int64
	var order int
	err = tx.QueryRowContext(c.Request.Context(), `SELECT r.rider_id,r.proposed_cents,r.status,d.order_id,d.delivery_status FROM rider_cancellation_pay_reviews r JOIN delivery_orders d USING(delivery_order_id) WHERE r.id=$1 FOR UPDATE OF r,d`, id).Scan(&rider, &cents, &status, &order, &deliveryStatus)
	if err != nil {
		dto.NotFound(c, "Review not found")
		return
	}
	if status != "pending" || deliveryStatus != "cancelled" {
		dto.Conflict(c, "Only cancelled deliveries with pending reviews can be reviewed")
		return
	}
	actor := middleware.GetUserID(c)
	status = "rejected"
	if *req.Approve {
		status = "approved"
		ref := "rider-cancellation:" + strconv.FormatInt(id, 10)
		_, err = repository.PostWalletTransaction(c.Request.Context(), tx, rider, models.WalletTxnIncentive, float64(cents)/100, &order, &ref, nil, &req.Note, &actor)
		if err != nil {
			dto.InternalError(c, "Cancellation payment could not be posted")
			return
		}
	}
	if _, err = tx.ExecContext(c.Request.Context(), `UPDATE rider_cancellation_pay_reviews SET status=$2,reviewed_by=$3,review_note=$4,reviewed_at=now() WHERE id=$1`, id, status, actor, req.Note); err != nil {
		dto.InternalError(c, "Review could not be saved")
		return
	}
	if tx.Commit() != nil {
		dto.InternalError(c, "Review could not be saved")
		return
	}
	dto.Success(c, 200, "Cancellation review saved", gin.H{"status": status})
}

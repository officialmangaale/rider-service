package handler

import (
	"encoding/json"
	"fmt"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/riderpay"
	"github.com/gin-gonic/gin"
	"strings"
	"time"
)

// Salary is a separate, explicitly approved accrual to the rider wallet. The
// existing settlement workflow records the later cash/bank payout.
func (h *AdminRiderHandler) AccrueSalary(c *gin.Context) {
	if !h.ContractPayEnabled {
		dto.Conflict(c, "Rider contract pay rollout is inactive")
		return
	}
	var req struct {
		Start string `json:"period_start"`
		Note  string `json:"note"`
	}
	if c.ShouldBindJSON(&req) != nil || strings.TrimSpace(req.Note) == "" || len(req.Note) > 1000 {
		dto.ValidationError(c, "Salary period start and approval note required")
		return
	}
	start, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		dto.ValidationError(c, "Use a valid salary period start date")
		return
	}
	rider := c.Param("riderId")
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		dto.InternalError(c, "Salary accrual unavailable")
		return
	}
	defer tx.Rollback()
	var raw []byte
	err = tx.QueryRowContext(c.Request.Context(), `SELECT to_jsonb(p) FROM rider_earning_contracts p WHERE rider_id=$1 AND enabled AND mode='salary' FOR UPDATE`, rider).Scan(&raw)
	if err != nil {
		dto.Conflict(c, "An enabled salary contract is required")
		return
	}
	var p riderpay.Policy
	if json.Unmarshal(raw, &p) != nil || p.Validate() != nil {
		dto.Conflict(c, "Invalid salary contract")
		return
	}
	end := start.AddDate(0, 0, 7)
	if p.SalaryPeriod == "monthly" {
		end = start.AddDate(0, 1, 0)
		if end.Day() != start.Day() {
			dto.ValidationError(c, "Choose a salary date that exists in the following month")
			return
		}
	}
	// Only completed salary periods can be approved. Date-only business periods
	// use IST; no order or cancellation automatically creates a salary accrual.
	ist := time.FixedZone("IST", 19800)
	periodEnd := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, ist)
	if time.Now().Before(periodEnd) {
		dto.ValidationError(c, "Salary period must be completed before accrual")
		return
	}
	var overlaps bool
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM rider_salary_accruals WHERE rider_id=$1 AND period_start<$3::date AND period_end>$2::date)`, rider, req.Start, end.Format("2006-01-02")).Scan(&overlaps); err != nil {
		dto.InternalError(c, "Salary history unavailable")
		return
	}
	if overlaps {
		dto.Conflict(c, "This salary period overlaps an existing accrual")
		return
	}
	var id int64
	actor := middleware.GetUserID(c)
	if err = tx.QueryRowContext(c.Request.Context(), `INSERT INTO rider_salary_accruals(rider_id,period_start,period_end,amount_cents,contract,created_by,note)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING id`, rider, req.Start, end.Format("2006-01-02"), p.SalaryCents, string(raw), actor, req.Note).Scan(&id); err != nil {
		dto.InternalError(c, "Salary accrual failed")
		return
	}
	reference := fmt.Sprintf("SALARY:%d", id)
	if _, err = repository.PostWalletTransaction(c.Request.Context(), tx, rider, models.WalletTxnManualAdjustment, float64(p.SalaryCents)/100, nil, &reference, nil, &req.Note, &actor); err != nil {
		dto.InternalError(c, "Salary credit failed")
		return
	}
	if tx.Commit() != nil {
		dto.InternalError(c, "Salary accrual failed")
		return
	}
	dto.Success(c, 201, "Salary accrued separately. Record the transfer using rider settlement.", gin.H{"id": id, "amount_cents": p.SalaryCents, "period_start": req.Start, "period_end": end.Format("2006-01-02")})
}

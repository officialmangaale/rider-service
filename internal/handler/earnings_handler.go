package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// EarningsHandler handles earnings endpoints.
type EarningsHandler struct {
	earningsSvc *service.EarningsService
	db          *sql.DB
}

// NewEarningsHandler creates a new EarningsHandler.
func NewEarningsHandler(earningsSvc *service.EarningsService, db *sql.DB) *EarningsHandler {
	return &EarningsHandler{earningsSvc: earningsSvc, db: db}
}

// GetSummary returns aggregated earnings.
func (h *EarningsHandler) GetSummary(c *gin.Context) {
	userID := middleware.GetUserID(c)
	summary, err := h.earningsSvc.GetSummary(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to fetch earnings summary")
		return
	}

	resp := dto.EarningsSummaryResponse{
		TodayEarnings:     summary.TodayEarnings,
		WeeklyEarnings:    summary.WeekEarnings,
		MonthlyEarnings:   summary.MonthEarnings,
		TotalEarnings:     summary.TotalEarnings,
		WalletBalance:     summary.WalletBalance,
		PendingPayout:     summary.PendingPayout,
		SettledPayout:     summary.SettledPayout,
		CompletedOrders:   summary.TotalOrders,
		DeliveryEarnings:  summary.DeliveryEarnings,
		TipEarnings:       summary.TipEarnings,
		IncentiveEarnings: summary.IncentiveEarnings,
		BonusEarnings:     summary.BonusEarnings,
		PenaltyAmount:     summary.PenaltyAmount,
	}

	dto.Success(c, http.StatusOK, "earnings summary", resp)
}

// GetHistory returns paginated earnings ledger.
func (h *EarningsHandler) GetHistory(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	earnings, total, err := h.earningsSvc.GetHistory(c.Request.Context(), userID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch earnings history")
		return
	}
	dto.Paginated(c, earnings, pq.Page, pq.Limit, total)
}

// GetWalletTransactions returns the authenticated rider's own wallet ledger
// (platform upgrade Module 21) — the same net-payable ledger Module 10's
// admin view reads, scoped to the caller's own rider ID instead of an
// admin-supplied one.
func (h *EarningsHandler) GetWalletTransactions(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	txns, total, err := repository.ListWalletTransactions(c.Request.Context(), h.db, userID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch wallet transactions")
		return
	}
	dto.Paginated(c, txns, pq.Page, pq.Limit, total)
}

// GetSettlements returns the authenticated rider's own settlement history
// (platform upgrade Module 21).
func (h *EarningsHandler) GetSettlements(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	settlements, total, err := repository.ListSettlements(c.Request.Context(), h.db, userID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch settlements")
		return
	}
	dto.Paginated(c, settlements, pq.Page, pq.Limit, total)
}

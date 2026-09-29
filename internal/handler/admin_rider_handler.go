package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// AdminRiderHandler serves the Module 10/11 admin rider-management endpoints.
type AdminRiderHandler struct {
	db *sql.DB
}

// NewAdminRiderHandler creates a new AdminRiderHandler.
func NewAdminRiderHandler(db *sql.DB) *AdminRiderHandler {
	return &AdminRiderHandler{db: db}
}

// ListRiders handles GET /api/v1/admin/riders
func (h *AdminRiderHandler) ListRiders(c *gin.Context) {
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	filter := repository.ListRidersFilter{
		Search:             c.Query("search"),
		OnlineOnly:         c.Query("online_only") == "true",
		NegativeWalletOnly: c.Query("negative_wallet_only") == "true",
		IncludeLocation:    c.Query("include_location") == "true",
		Limit:              pq.Limit,
		Offset:             pq.Offset(),
	}

	riders, total, err := repository.ListRiders(c.Request.Context(), h.db, filter)
	if err != nil {
		dto.InternalError(c, "Failed to fetch riders")
		return
	}
	dto.Paginated(c, riders, pq.Page, pq.Limit, total)
}

// GetWalletTransactions handles GET /api/v1/admin/riders/:riderId/wallet/transactions
func (h *AdminRiderHandler) GetWalletTransactions(c *gin.Context) {
	riderID := c.Param("riderId")
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	txns, total, err := repository.ListWalletTransactions(c.Request.Context(), h.db, riderID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch wallet transactions")
		return
	}
	dto.Paginated(c, txns, pq.Page, pq.Limit, total)
}

// GetSettlements handles GET /api/v1/admin/riders/:riderId/settlements
func (h *AdminRiderHandler) GetSettlements(c *gin.Context) {
	riderID := c.Param("riderId")
	var pq dto.PaginationQuery
	_ = c.ShouldBindQuery(&pq)
	pq.Normalize()

	settlements, total, err := repository.ListSettlements(c.Request.Context(), h.db, riderID, pq.Limit, pq.Offset())
	if err != nil {
		dto.InternalError(c, "Failed to fetch settlements")
		return
	}
	dto.Paginated(c, settlements, pq.Page, pq.Limit, total)
}

// parseAdminReportDateRange reads ?from/?to (YYYY-MM-DD) query params,
// defaulting to the last 30 days when both are absent (platform upgrade
// Module 24: Reports and Exports).
func parseAdminReportDateRange(c *gin.Context) (*time.Time, *time.Time, bool) {
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))
	if fromStr == "" && toStr == "" {
		to := time.Now().UTC()
		from := to.AddDate(0, -1, 0)
		return &from, &to, true
	}
	var from, to *time.Time
	if fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			dto.ValidationError(c, "invalid from date (use YYYY-MM-DD)")
			return nil, nil, false
		}
		from = &t
	}
	if toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			dto.ValidationError(c, "invalid to date (use YYYY-MM-DD)")
			return nil, nil, false
		}
		endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		to = &endOfDay
	}
	return from, to, true
}

// ExportWalletTransactionsCSV handles
// GET /api/v1/admin/reports/riders/wallet-transactions/export-csv (platform
// upgrade Module 24: Reports and Exports). Cross-rider — reuses
// ListAllWalletTransactions. Filters: rider_id, from, to (YYYY-MM-DD,
// defaults to last 30 days).
func (h *AdminRiderHandler) ExportWalletTransactionsCSV(c *gin.Context) {
	from, to, ok := parseAdminReportDateRange(c)
	if !ok {
		return
	}
	filter := repository.AllWalletTransactionsFilter{
		RiderID:  strings.TrimSpace(c.Query("rider_id")),
		DateFrom: from,
		DateTo:   to,
		Limit:    5000,
	}
	txns, err := repository.ListAllWalletTransactions(c.Request.Context(), h.db, filter)
	if err != nil {
		dto.InternalError(c, "Failed to fetch wallet transactions")
		return
	}

	rows := make([][]string, 0, len(txns))
	for _, t := range txns {
		orderID := ""
		if t.OrderID != nil {
			orderID = strconv.Itoa(*t.OrderID)
		}
		rows = append(rows, []string{
			strconv.FormatInt(t.ID, 10),
			t.RiderID,
			t.TransactionType,
			strconv.FormatFloat(t.Amount, 'f', 2, 64),
			strconv.FormatFloat(t.BalanceAfter, 'f', 2, 64),
			orderID,
			t.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	dto.WriteCSV(c, "riders-wallet-report",
		[]string{"Transaction ID", "Rider ID", "Type", "Amount", "Balance After", "Order ID", "Created At"},
		rows)
}

// ExportSettlementsCSV handles
// GET /api/v1/admin/reports/riders/settlements/export-csv (platform upgrade
// Module 24: Reports and Exports). Cross-rider — reuses ListAllSettlements.
// Filters: rider_id, from, to (YYYY-MM-DD, defaults to last 30 days).
func (h *AdminRiderHandler) ExportSettlementsCSV(c *gin.Context) {
	from, to, ok := parseAdminReportDateRange(c)
	if !ok {
		return
	}
	filter := repository.AllSettlementsFilter{
		RiderID:  strings.TrimSpace(c.Query("rider_id")),
		DateFrom: from,
		DateTo:   to,
		Limit:    5000,
	}
	settlements, err := repository.ListAllSettlements(c.Request.Context(), h.db, filter)
	if err != nil {
		dto.InternalError(c, "Failed to fetch settlements")
		return
	}

	rows := make([][]string, 0, len(settlements))
	for _, s := range settlements {
		rows = append(rows, []string{
			strconv.FormatInt(s.ID, 10),
			s.RiderID,
			strconv.FormatFloat(s.PreviousBalance, 'f', 2, 64),
			strconv.FormatFloat(s.SettlementAmount, 'f', 2, 64),
			strconv.FormatFloat(s.AmountReceived, 'f', 2, 64),
			strconv.FormatFloat(s.AmountPaid, 'f', 2, 64),
			strconv.FormatFloat(s.ClosingBalance, 'f', 2, 64),
			strconv.FormatBool(s.IsFullAndFinal),
			s.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	dto.WriteCSV(c, "riders-settlements-report",
		[]string{"Settlement ID", "Rider ID", "Previous Balance", "Settlement Amount", "Amount Received",
			"Amount Paid", "Closing Balance", "Full And Final", "Created At"},
		rows)
}

// CreateSettlementRequest is the POST body for recording a settlement.
type CreateSettlementRequest struct {
	AmountReceived   float64 `json:"amount_received"`
	AmountPaid       float64 `json:"amount_paid"`
	AdjustmentAmount float64 `json:"adjustment_amount"`
	PaymentMode      *string `json:"payment_mode"`
	ReferenceNumber  *string `json:"reference_number"`
	IsFullAndFinal   bool    `json:"is_full_and_final"`
	Notes            *string `json:"notes"`
}

// CreateSettlement handles POST /api/v1/admin/riders/:riderId/settlements
func (h *AdminRiderHandler) CreateSettlement(c *gin.Context) {
	riderID := c.Param("riderId")
	adminID := middleware.GetUserID(c)

	var req CreateSettlementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}
	if req.AmountReceived < 0 || req.AmountPaid < 0 {
		dto.ValidationError(c, "amount_received and amount_paid must not be negative")
		return
	}
	if req.AmountReceived == 0 && req.AmountPaid == 0 && req.AdjustmentAmount == 0 {
		dto.ValidationError(c, "Settlement must have a non-zero received, paid, or adjustment amount")
		return
	}

	settlement, err := repository.CreateSettlement(c.Request.Context(), h.db, repository.SettlementInput{
		RiderID:          riderID,
		AmountReceived:   req.AmountReceived,
		AmountPaid:       req.AmountPaid,
		AdjustmentAmount: req.AdjustmentAmount,
		PaymentMode:      req.PaymentMode,
		ReferenceNumber:  req.ReferenceNumber,
		IsFullAndFinal:   req.IsFullAndFinal,
		Notes:            req.Notes,
		AdminID:          adminID,
	})
	if err != nil {
		dto.InternalError(c, "Failed to record settlement")
		return
	}
	dto.Success(c, http.StatusCreated, "settlement recorded", settlement)
}

// CreateWalletAdjustmentRequest is the POST body for a standalone wallet
// entry (an incentive, penalty, or manual correction) outside a settlement.
type CreateWalletAdjustmentRequest struct {
	TransactionType string  `json:"transaction_type"`
	Amount          float64 `json:"amount"` // magnitude for directional types below; signed for manual_adjustment/refund_reversal
	ReferenceNumber *string `json:"reference_number"`
	Notes           *string `json:"notes"`
}

// CreateWalletAdjustment handles POST /api/v1/admin/riders/:riderId/wallet/adjustments.
//
// Directional types (incentive, penalty, rider_payment_to_company,
// company_payment_to_rider, cod_liability) take a positive magnitude and the
// sign is applied here, matching the same convention CreateSettlement uses
// for amount_received/amount_paid — so an admin cannot flip a type's meaning
// by fat-fingering a sign. manual_adjustment and refund_reversal are
// corrections with no fixed direction, so their amount is used as given.
func (h *AdminRiderHandler) CreateWalletAdjustment(c *gin.Context) {
	riderID := c.Param("riderId")
	adminID := middleware.GetUserID(c)

	var req CreateWalletAdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "Invalid request body")
		return
	}

	var signedAmount float64
	switch req.TransactionType {
	case models.WalletTxnIncentive, models.WalletTxnRiderPaymentToCompany:
		if req.Amount <= 0 {
			dto.ValidationError(c, "amount must be positive for this transaction type")
			return
		}
		signedAmount = req.Amount
	case models.WalletTxnPenalty, models.WalletTxnCompanyPaymentToRider, models.WalletTxnCODLiability:
		if req.Amount <= 0 {
			dto.ValidationError(c, "amount must be positive for this transaction type")
			return
		}
		signedAmount = -req.Amount
	case models.WalletTxnManualAdjustment, models.WalletTxnRefundReversal:
		if req.Amount == 0 {
			dto.ValidationError(c, "amount must not be zero")
			return
		}
		signedAmount = req.Amount
	default:
		dto.ValidationError(c, "Unsupported transaction_type for a manual adjustment")
		return
	}

	txn, err := repository.PostStandaloneWalletTransaction(c.Request.Context(), h.db, riderID, req.TransactionType,
		signedAmount, req.ReferenceNumber, req.Notes, &adminID)
	if err != nil {
		dto.InternalError(c, "Failed to record wallet adjustment")
		return
	}
	dto.Success(c, http.StatusCreated, "wallet adjustment recorded", txn)
}

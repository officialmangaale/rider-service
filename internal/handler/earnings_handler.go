package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// EarningsHandler handles earnings endpoints.
type EarningsHandler struct {
	earningsSvc *service.EarningsService
}

// NewEarningsHandler creates a new EarningsHandler.
func NewEarningsHandler(earningsSvc *service.EarningsService) *EarningsHandler {
	return &EarningsHandler{earningsSvc: earningsSvc}
}

// GetSummary returns aggregated earnings.
func (h *EarningsHandler) GetSummary(c *gin.Context) {
	userID := middleware.GetUserID(c)
	summary, err := h.earningsSvc.GetSummary(c.Request.Context(), userID)
	if err != nil {
		dto.InternalError(c, "Failed to fetch earnings summary")
		return
	}
	dto.Success(c, http.StatusOK, "earnings summary", summary)
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

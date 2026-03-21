package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	db *sql.DB
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health performs a basic health check including DB ping.
func (h *HealthHandler) Health(c *gin.Context) {
	dbOK := "up"
	if err := h.db.Ping(); err != nil {
		dbOK = "down"
	}
	dto.Success(c, http.StatusOK, "rider-service healthy", gin.H{
		"service":  "rider-service",
		"database": dbOK,
	})
}

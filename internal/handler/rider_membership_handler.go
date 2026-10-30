package handler

import (
	"database/sql"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"strconv"
)

type RiderMembershipHandler struct{ db *sql.DB }

func NewRiderMembershipHandler(db *sql.DB) *RiderMembershipHandler {
	return &RiderMembershipHandler{db: db}
}
func (h *RiderMembershipHandler) Profile(c *gin.Context) {
	rider := middleware.GetUserID(c)
	_, err := h.db.ExecContext(c.Request.Context(), `INSERT INTO rider_work_profiles(rider_id) SELECT id::text FROM users WHERE id::text=$1 AND primary_role IN ('rider','delivery_driver') AND deleted_at IS NULL ON CONFLICT DO NOTHING`, rider)
	if err != nil {
		dto.ErrorResponse(c, 503, "Rider membership unavailable", "")
		return
	}
	var code string
	var work *string
	err = h.db.QueryRowContext(c.Request.Context(), `SELECT rider_code,work_type FROM rider_work_profiles WHERE rider_id=$1`, rider).Scan(&code, &work)
	if err != nil {
		dto.NotFound(c, "Rider not found")
		return
	}
	dto.Success(c, 200, "Rider membership", gin.H{"rider_code": code, "work_type": work})
}
func (h *RiderMembershipHandler) WorkType(c *gin.Context) {
	var req struct {
		Type string `json:"work_type"`
	}
	if c.ShouldBindJSON(&req) != nil || (req.Type != "part_time" && req.Type != "full_time") {
		dto.ValidationError(c, "Choose part-time or full-time")
		return
	}
	result, err := h.db.ExecContext(c.Request.Context(), `UPDATE users SET rider_work_type=$2 WHERE id::text=$1 AND primary_role IN ('rider','delivery_driver') AND deleted_at IS NULL`, middleware.GetUserID(c), req.Type)
	if err != nil {
		dto.ErrorResponse(c, 503, "Work type could not be saved", "")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		dto.NotFound(c, "Rider not found")
		return
	}
	dto.Success(c, 200, "Work type saved", nil)
}
func (h *RiderMembershipHandler) Invitations(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT i.invitation_id,i.restaurant_id,r.name,i.expires_at FROM restaurant_rider_invitations i JOIN restaurants r ON r.restaurant_id=i.restaurant_id WHERE i.rider_id=$1 AND i.status='pending' AND i.expires_at>now() ORDER BY i.created_at DESC LIMIT 100`, middleware.GetUserID(c))
	if err != nil {
		dto.ErrorResponse(c, 503, "Invitations unavailable", "")
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, restaurant int64
		var name string
		var expires interface{}
		if err = rows.Scan(&id, &restaurant, &name, &expires); err != nil {
			dto.ErrorResponse(c, 503, "Invitations unavailable", "")
			return
		}
		list = append(list, gin.H{"invitation_id": id, "restaurant_id": restaurant, "restaurant_name": name, "expires_at": expires})
	}
	if rows.Err() != nil {
		dto.ErrorResponse(c, 503, "Invitations unavailable", "")
		return
	}
	dto.Success(c, 200, "Restaurant invitations", list)
}
func (h *RiderMembershipHandler) Answer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("invitationId"), 10, 64)
	var req struct {
		Accept *bool `json:"accept"`
	}
	if err != nil || id <= 0 || c.ShouldBindJSON(&req) != nil || req.Accept == nil {
		dto.ValidationError(c, "Invitation and accept decision required")
		return
	}
	_, err = h.db.ExecContext(c.Request.Context(), `SELECT mangaale_answer_rider_invitation($1,$2,$3)`, id, middleware.GetUserID(c), *req.Accept)
	if err != nil {
		if p, ok := err.(*pq.Error); ok && p.Code == "P1003" {
			dto.Conflict(c, p.Message)
		} else {
			dto.ErrorResponse(c, 503, "Invitation could not be answered", "")
		}
		return
	}
	dto.Success(c, 200, "Invitation answered", nil)
}

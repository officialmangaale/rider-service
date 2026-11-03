package handler

import (
	"database/sql"
	"errors"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"strings"
)

func (h *RiderMembershipHandler) Reapply(c *gin.Context) {
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		dto.InternalError(c, "Application unavailable")
		return
	}
	defer tx.Rollback()
	rider := middleware.GetUserID(c)
	var locked string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT rider_id FROM rider_work_profiles WHERE rider_id=$1 FOR UPDATE`, rider).Scan(&locked); err != nil {
		dto.NotFound(c, "Rider not found")
		return
	}
	var state string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT state FROM rider_platform_access WHERE rider_id=$1 FOR UPDATE`, rider).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			dto.Conflict(c, "No platform re-application is required")
		} else {
			dto.InternalError(c, "Application unavailable")
		}
		return
	}
	var linked bool
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM restaurant_riders WHERE rider_user_id=$1 AND is_active AND COALESCE(status,'active')='active')`, rider).Scan(&linked); err != nil {
		dto.InternalError(c, "Application unavailable")
		return
	}
	if linked {
		dto.Conflict(c, "Only the restaurant owner can remove your restaurant link before re-application")
		return
	}
	if state == "approved" {
		dto.Conflict(c, "You are already approved for platform work")
		return
	}
	if state != "pending" {
		_, err = tx.ExecContext(c.Request.Context(), `UPDATE rider_platform_access SET state='pending',requested_at=now(),reviewed_at=NULL,reviewed_by=NULL,review_note='',updated_at=now() WHERE rider_id=$1`, rider)
		if err == nil {
			_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO rider_platform_access_audit(rider_id,state,restaurant_id,actor_id) SELECT rider_id,'pending',last_restaurant_id,$1 FROM rider_platform_access WHERE rider_id=$1`, rider)
		}
		if err != nil {
			dto.InternalError(c, "Application could not be submitted")
			return
		}
	}
	if tx.Commit() != nil {
		dto.InternalError(c, "Application could not be submitted")
		return
	}
	dto.Success(c, 200, "Application submitted. Wait for admin approval before going online.", gin.H{"platform_status": "pending"})
}

func (h *AdminRiderHandler) PlatformApplications(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT p.rider_id,w.rider_code,COALESCE(NULLIF(u.display_name,''),concat_ws(' ',u.first_name,u.last_name)),p.state,p.last_restaurant_id,COALESCE(r.name,''),p.requested_at,p.reviewed_at,p.review_note
 FROM rider_platform_access p JOIN rider_work_profiles w USING(rider_id) JOIN users u ON u.id::text=p.rider_id LEFT JOIN restaurants r ON r.restaurant_id=p.last_restaurant_id
 WHERE u.deleted_at IS NULL ORDER BY (p.state='pending') DESC,p.updated_at DESC LIMIT 200`)
	if err != nil {
		dto.InternalError(c, "Applications unavailable")
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var rider, code, name, state, restaurantName, note string
		var restaurantID sql.NullInt64
		var requested, reviewed sql.NullTime
		if rows.Scan(&rider, &code, &name, &state, &restaurantID, &restaurantName, &requested, &reviewed, &note) != nil {
			dto.InternalError(c, "Applications unavailable")
			return
		}
		out = append(out, gin.H{"rider_id": rider, "rider_code": code, "name": name, "state": state, "last_restaurant_id": restaurantID.Int64, "last_restaurant_name": restaurantName, "requested_at": requested.Time, "reviewed_at": reviewed.Time, "review_note": note})
	}
	if rows.Err() != nil {
		dto.InternalError(c, "Applications unavailable")
		return
	}
	dto.Success(c, 200, "Platform applications", out)
}

func (h *AdminRiderHandler) ReviewPlatformApplication(c *gin.Context) {
	var req struct {
		Approve *bool  `json:"approve"`
		Note    string `json:"note"`
	}
	rider := strings.TrimSpace(c.Param("riderId"))
	if rider == "" || c.ShouldBindJSON(&req) != nil || req.Approve == nil || len(strings.TrimSpace(req.Note)) == 0 || len(req.Note) > 1000 {
		dto.ValidationError(c, "Decision and review note are required")
		return
	}
	tx, err := h.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		dto.InternalError(c, "Review unavailable")
		return
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT rider_id FROM rider_work_profiles WHERE rider_id=$1 FOR UPDATE`, rider).Scan(&locked); err != nil {
		dto.NotFound(c, "Rider not found")
		return
	}
	var state string
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT state FROM rider_platform_access WHERE rider_id=$1 FOR UPDATE`, rider).Scan(&state); err != nil {
		dto.NotFound(c, "Application not found")
		return
	}
	if state != "pending" {
		dto.Conflict(c, "Only pending applications can be reviewed")
		return
	}
	var linked bool
	if err = tx.QueryRowContext(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM restaurant_riders WHERE rider_user_id=$1 AND is_active AND COALESCE(status,'active')='active')`, rider).Scan(&linked); err != nil {
		dto.InternalError(c, "Review unavailable")
		return
	}
	if linked {
		dto.Conflict(c, "Rider is currently linked to a restaurant")
		return
	}
	state = "rejected"
	body := "Your platform application was not approved. Check your review and re-apply when ready."
	if *req.Approve {
		state = "approved"
		body = "Your platform application is approved. You can now go online for platform orders."
	}
	_, err = tx.ExecContext(c.Request.Context(), `UPDATE rider_platform_access SET state=$2,reviewed_at=now(),reviewed_by=$3,review_note=$4,updated_at=now() WHERE rider_id=$1`, rider, state, middleware.GetUserID(c), strings.TrimSpace(req.Note))
	if err == nil {
		_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO rider_platform_access_audit(rider_id,state,restaurant_id,actor_id,note) SELECT rider_id,state,last_restaurant_id,$2,review_note FROM rider_platform_access WHERE rider_id=$1`, rider, middleware.GetUserID(c))
	}
	if err == nil {
		_, err = tx.ExecContext(c.Request.Context(), `INSERT INTO rider_notifications(rider_id,title,body,type,data) VALUES($1::uuid,'Platform application reviewed',$2,'platform_reapplication',jsonb_build_object('state',$3::text))`, rider, body, state)
	}
	if err != nil || tx.Commit() != nil {
		dto.InternalError(c, "Review could not be saved")
		return
	}
	dto.Success(c, 200, "Application reviewed", gin.H{"platform_status": state})
}

package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
)

const (
	maxTicketSubjectRunes     = 120
	maxTicketDescriptionRunes = 2000
	maxTicketsPerRiderPerDay  = 10
	duplicateTicketWindow     = 5 * time.Minute
)

var (
	ticketCategories = map[string]bool{"delivery": true, "payout": true, "account": true, "safety": true, "other": true}
	ticketStatuses   = map[string]bool{"open": true, "in_progress": true, "resolved": true, "closed": true}
)

// SupportHandler serves rider support: tickets raised from the app, the
// operations contact details, and the admin view for handling tickets.
type SupportHandler struct {
	db *sql.DB
}

func NewSupportHandler(db *sql.DB) *SupportHandler { return &SupportHandler{db: db} }

type createTicketRequest struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Category    string `json:"category"`
	OrderID     *int64 `json:"order_id"`
}

type ticketView struct {
	ID          int64      `json:"id"`
	Category    string     `json:"category"`
	Subject     string     `json:"subject"`
	Description string     `json:"description"`
	OrderID     *int64     `json:"order_id,omitempty"`
	Status      string     `json:"status"`
	AdminNote   string     `json:"admin_note,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

const ticketColumns = `id, category, subject, description, order_id, status, COALESCE(admin_note,''), created_at, updated_at, resolved_at`

func (t *ticketView) scanTargets() []any {
	return []any{&t.ID, &t.Category, &t.Subject, &t.Description, &t.OrderID, &t.Status, &t.AdminNote, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt}
}

// CreateTicket handles POST /api/v1/support/tickets.
func (h *SupportHandler) CreateTicket(c *gin.Context) {
	riderID := middleware.GetUserID(c)
	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "subject and description are required")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Description = strings.TrimSpace(req.Description)
	req.Category = strings.ToLower(strings.TrimSpace(req.Category))
	if req.Category == "" {
		req.Category = "other"
	}
	if n := utf8.RuneCountInString(req.Subject); n == 0 || n > maxTicketSubjectRunes {
		dto.ValidationError(c, "subject is required (max 120 characters)")
		return
	}
	if n := utf8.RuneCountInString(req.Description); n == 0 || n > maxTicketDescriptionRunes {
		dto.ValidationError(c, "description is required (max 2000 characters)")
		return
	}
	if !ticketCategories[req.Category] {
		dto.ValidationError(c, "category must be delivery, payout, account, safety or other")
		return
	}
	ctx := c.Request.Context()

	// An order reference must be one this rider actually handled.
	if req.OrderID != nil {
		var ok bool
		err := h.db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM delivery_orders
			WHERE order_id = $1 AND (assigned_rider_id = $2 OR rider_user_id = $2))`, *req.OrderID, riderID).Scan(&ok)
		if err != nil {
			dto.InternalError(c, "Failed to create ticket")
			return
		}
		if !ok {
			dto.ValidationError(c, "order_id is not one of your deliveries")
			return
		}
	}

	// A double tap or a retry after a lost response returns the first ticket.
	var existing ticketView
	err := h.db.QueryRowContext(ctx, `SELECT `+ticketColumns+`
		FROM rider_support_tickets
		WHERE rider_id = $1 AND subject = $2 AND description = $3 AND created_at > NOW() - make_interval(secs => $4)
		ORDER BY id DESC LIMIT 1`, riderID, req.Subject, req.Description, duplicateTicketWindow.Seconds()).
		Scan(existing.scanTargets()...)
	if err == nil {
		dto.Success(c, http.StatusOK, "ticket already created", existing)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		dto.InternalError(c, "Failed to create ticket")
		return
	}

	var today int
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rider_support_tickets WHERE rider_id = $1 AND created_at > NOW() - INTERVAL '24 hours'`, riderID).Scan(&today); err != nil {
		dto.InternalError(c, "Failed to create ticket")
		return
	}
	if today >= maxTicketsPerRiderPerDay {
		dto.ErrorWithCode(c, http.StatusTooManyRequests, "You have raised several tickets today. Please wait for support to respond.", "TOO_MANY_TICKETS", nil)
		return
	}

	var created ticketView
	err = h.db.QueryRowContext(ctx, `INSERT INTO rider_support_tickets (rider_id, category, subject, description, order_id)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING `+ticketColumns, riderID, req.Category, req.Subject, req.Description, req.OrderID).
		Scan(created.scanTargets()...)
	if err != nil {
		dto.InternalError(c, "Failed to create ticket")
		return
	}
	dto.Success(c, http.StatusCreated, "ticket created", created)
}

// ListMyTickets handles GET /api/v1/support/tickets (the caller's own, newest first).
func (h *SupportHandler) ListMyTickets(c *gin.Context) {
	rows, err := h.db.QueryContext(c.Request.Context(), `SELECT `+ticketColumns+`
		FROM rider_support_tickets WHERE rider_id = $1 ORDER BY created_at DESC LIMIT 50`, middleware.GetUserID(c))
	if err != nil {
		dto.InternalError(c, "Failed to load tickets")
		return
	}
	defer rows.Close()
	out := []ticketView{}
	for rows.Next() {
		var t ticketView
		if err := rows.Scan(t.scanTargets()...); err != nil {
			dto.InternalError(c, "Failed to load tickets")
			return
		}
		out = append(out, t)
	}
	dto.Success(c, http.StatusOK, "tickets", gin.H{"tickets": out})
}

// Contact handles GET /api/v1/support/contact. The numbers come from the
// environment (SUPPORT_PHONE, SUPPORT_EMAIL, EMERGENCY_PHONE); any that is not
// configured is returned empty so the app can hide the control instead of
// showing a button that goes nowhere.
func (h *SupportHandler) Contact(c *gin.Context) {
	dto.Success(c, http.StatusOK, "support contact", gin.H{
		"support_phone":   strings.TrimSpace(os.Getenv("SUPPORT_PHONE")),
		"support_email":   strings.TrimSpace(os.Getenv("SUPPORT_EMAIL")),
		"emergency_phone": strings.TrimSpace(os.Getenv("EMERGENCY_PHONE")),
	})
}

// AdminListTickets handles GET /api/v1/admin/support-tickets?status=&limit=&offset=.
// Open and in-progress tickets come first.
func (h *SupportHandler) AdminListTickets(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !ticketStatuses[status] {
		dto.ValidationError(c, "status must be open, in_progress, resolved or closed")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := h.db.QueryContext(c.Request.Context(), `
		SELECT t.id, t.category, t.subject, t.description, t.order_id, t.status, COALESCE(t.admin_note,''),
		       t.created_at, t.updated_at, t.resolved_at,
		       t.rider_id::text,
		       COALESCE(NULLIF(TRIM(CONCAT(u.first_name,' ',u.last_name)),''), u.display_name, ''),
		       COALESCE(u.phone,'')
		FROM rider_support_tickets t JOIN users u ON u.id = t.rider_id
		WHERE ($1 = '' OR t.status = $1)
		ORDER BY (t.status IN ('open','in_progress')) DESC, t.created_at DESC
		LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		dto.InternalError(c, "Failed to load tickets")
		return
	}
	defer rows.Close()

	type adminTicket struct {
		ticketView
		RiderID    string `json:"rider_id"`
		RiderName  string `json:"rider_name"`
		RiderPhone string `json:"rider_phone"`
	}
	out := []adminTicket{}
	for rows.Next() {
		var t adminTicket
		dest := append(t.scanTargets(), &t.RiderID, &t.RiderName, &t.RiderPhone)
		if err := rows.Scan(dest...); err != nil {
			dto.InternalError(c, "Failed to load tickets")
			return
		}
		out = append(out, t)
	}
	dto.Success(c, http.StatusOK, "tickets", gin.H{"tickets": out})
}

// AdminUpdateTicket handles PATCH /api/v1/admin/support-tickets/:id.
func (h *SupportHandler) AdminUpdateTicket(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		dto.ValidationError(c, "invalid ticket id")
		return
	}
	var req struct {
		Status    string  `json:"status"`
		AdminNote *string `json:"admin_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ValidationError(c, "status is required")
		return
	}
	req.Status = strings.TrimSpace(req.Status)
	if !ticketStatuses[req.Status] {
		dto.ValidationError(c, "status must be open, in_progress, resolved or closed")
		return
	}
	if req.AdminNote != nil && utf8.RuneCountInString(*req.AdminNote) > maxTicketDescriptionRunes {
		dto.ValidationError(c, "admin_note is too long")
		return
	}
	res, err := h.db.ExecContext(c.Request.Context(), `UPDATE rider_support_tickets
		SET status = $2,
		    admin_note = COALESCE($3, admin_note),
		    handled_by = $4::uuid,
		    updated_at = NOW(),
		    resolved_at = CASE WHEN $2 IN ('resolved','closed') THEN COALESCE(resolved_at, NOW()) ELSE NULL END
		WHERE id = $1`, id, req.Status, req.AdminNote, middleware.GetUserID(c))
	if err != nil {
		dto.InternalError(c, "Failed to update ticket")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		dto.NotFound(c, "ticket not found")
		return
	}
	dto.Success(c, http.StatusOK, "ticket updated", gin.H{"id": id, "status": req.Status})
}

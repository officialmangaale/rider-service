package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

const (
	supRider1 = "d1000000-0000-4000-8000-000000000001"
	supRider2 = "d1000000-0000-4000-8000-000000000002"
	supAdmin  = "d1000000-0000-4000-8000-0000000000ad"
)

// supportRig is the support routes on a real PostgreSQL with the current
// schema plus migration 012, acting as whichever user a request names in the
// X-Test-User / X-Test-Role headers (standing in for the JWT middleware).
type supportRig struct {
	db     *sql.DB
	router *gin.Engine
}

func newSupportRig(t *testing.T) *supportRig {
	t.Helper()
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatalf("lifecycle schema: %v", err)
	}
	ddl, err := os.ReadFile("../../migrations/012_rider_support_tickets.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(ddl)); err != nil {
		t.Fatalf("migration 012: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, first_name, last_name, phone, primary_role) VALUES
		($1,'Asha','Rider','9000000001','delivery_driver'),
		($2,'Bharat','Rider','9000000002','delivery_driver'),
		($3,'Ops','Admin','9000000003','admin')`, supRider1, supRider2, supAdmin); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", c.GetHeader("X-Test-User"))
		c.Set("user_role", c.GetHeader("X-Test-Role"))
		c.Next()
	})
	h := NewSupportHandler(db)
	r.POST("/support/tickets", h.CreateTicket)
	r.GET("/support/tickets", h.ListMyTickets)
	r.GET("/support/contact", h.Contact)
	admin := r.Group("/admin/support-tickets", middleware.RequireAdmin())
	admin.GET("", h.AdminListTickets)
	admin.PATCH("/:id", h.AdminUpdateTicket)
	return &supportRig{db: db, router: r}
}

func (s *supportRig) do(t *testing.T, method, path, user, role string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	req.Header.Set("X-Test-Role", role)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func dataOf(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)
	return d
}

func TestSupportTicketLifecycle(t *testing.T) {
	s := newSupportRig(t)

	// A rider raises a payout ticket.
	code, res := s.do(t, "POST", "/support/tickets", supRider1, "delivery_driver",
		map[string]any{"subject": "Payout missing", "description": "COD settled but wallet not updated", "category": "payout"})
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, res)
	}
	id := dataOf(res)["id"].(float64)
	if dataOf(res)["status"] != "open" || dataOf(res)["category"] != "payout" {
		t.Fatalf("ticket = %v", dataOf(res))
	}

	// A double tap returns the same ticket instead of filing a second one.
	code, res = s.do(t, "POST", "/support/tickets", supRider1, "delivery_driver",
		map[string]any{"subject": "Payout missing", "description": "COD settled but wallet not updated", "category": "payout"})
	if code != http.StatusOK || dataOf(res)["id"].(float64) != id {
		t.Fatalf("duplicate = %d %v, want the first ticket back", code, res)
	}
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM rider_support_tickets`).Scan(&n)
	if n != 1 {
		t.Fatalf("tickets in db = %d, want 1", n)
	}

	// Riders see only their own tickets.
	_, res = s.do(t, "GET", "/support/tickets", supRider2, "delivery_driver", nil)
	if list := dataOf(res)["tickets"].([]any); len(list) != 0 {
		t.Fatalf("rider 2 sees %d tickets, want 0", len(list))
	}
	_, res = s.do(t, "GET", "/support/tickets", supRider1, "delivery_driver", nil)
	if list := dataOf(res)["tickets"].([]any); len(list) != 1 {
		t.Fatalf("rider 1 sees %d tickets, want 1", len(list))
	}

	// Riders cannot use the admin queue.
	if code, _ := s.do(t, "GET", "/admin/support-tickets", supRider1, "delivery_driver", nil); code != http.StatusForbidden {
		t.Fatalf("rider on admin list = %d, want 403", code)
	}
	if code, _ := s.do(t, "PATCH", "/admin/support-tickets/1", supRider1, "delivery_driver", map[string]any{"status": "resolved"}); code != http.StatusForbidden {
		t.Fatalf("rider on admin update = %d, want 403", code)
	}

	// Admin sees it with the rider's name and phone, then resolves it.
	_, res = s.do(t, "GET", "/admin/support-tickets?status=open", supAdmin, "admin", nil)
	list := dataOf(res)["tickets"].([]any)
	if len(list) != 1 {
		t.Fatalf("admin open list = %d, want 1", len(list))
	}
	first := list[0].(map[string]any)
	if first["rider_name"] != "Asha Rider" || first["rider_phone"] != "9000000001" {
		t.Fatalf("admin row = %v", first)
	}
	code, _ = s.do(t, "PATCH", "/admin/support-tickets/1", supAdmin, "admin",
		map[string]any{"status": "resolved", "admin_note": "Wallet corrected"})
	if code != http.StatusOK {
		t.Fatalf("resolve = %d", code)
	}
	_, res = s.do(t, "GET", "/support/tickets", supRider1, "delivery_driver", nil)
	mine := dataOf(res)["tickets"].([]any)[0].(map[string]any)
	if mine["status"] != "resolved" || mine["admin_note"] != "Wallet corrected" || mine["resolved_at"] == nil {
		t.Fatalf("rider's view after resolve = %v", mine)
	}

	// Reopening clears resolved_at; bad input is refused; unknown ids are 404.
	s.do(t, "PATCH", "/admin/support-tickets/1", supAdmin, "admin", map[string]any{"status": "in_progress"})
	var resolvedAt sql.NullTime
	_ = s.db.QueryRow(`SELECT resolved_at FROM rider_support_tickets WHERE id=1`).Scan(&resolvedAt)
	if resolvedAt.Valid {
		t.Fatal("reopening must clear resolved_at")
	}
	if code, _ := s.do(t, "PATCH", "/admin/support-tickets/1", supAdmin, "admin", map[string]any{"status": "banana"}); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d, want 400", code)
	}
	if code, _ := s.do(t, "PATCH", "/admin/support-tickets/999", supAdmin, "admin", map[string]any{"status": "closed"}); code != http.StatusNotFound {
		t.Fatalf("unknown ticket = %d, want 404", code)
	}
}

func TestSupportTicketValidation(t *testing.T) {
	s := newSupportRig(t)
	post := func(body map[string]any) int {
		code, _ := s.do(t, "POST", "/support/tickets", supRider1, "delivery_driver", body)
		return code
	}
	if c := post(map[string]any{"subject": "  ", "description": "x"}); c != http.StatusBadRequest {
		t.Errorf("blank subject = %d, want 400", c)
	}
	if c := post(map[string]any{"subject": "x", "description": ""}); c != http.StatusBadRequest {
		t.Errorf("blank description = %d, want 400", c)
	}
	if c := post(map[string]any{"subject": strings.Repeat("s", 121), "description": "x"}); c != http.StatusBadRequest {
		t.Errorf("long subject = %d, want 400", c)
	}
	if c := post(map[string]any{"subject": "x", "description": strings.Repeat("d", 2001)}); c != http.StatusBadRequest {
		t.Errorf("long description = %d, want 400", c)
	}
	if c := post(map[string]any{"subject": "x", "description": "y", "category": "gossip"}); c != http.StatusBadRequest {
		t.Errorf("bad category = %d, want 400", c)
	}

	// An order reference must be a delivery this rider handled.
	if _, err := s.db.Exec(`INSERT INTO delivery_orders (order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude,
		drop_latitude, drop_longitude, delivery_status, assigned_rider_id, rider_user_id)
		VALUES (555, 27, 1, 28.4, 77.0, 28.5, 77.1, 'delivered', $1, $1)`, supRider1); err != nil {
		t.Fatal(err)
	}
	if c := post(map[string]any{"subject": "Order issue", "description": "customer absent", "order_id": 555}); c != http.StatusCreated {
		t.Errorf("own order reference = %d, want 201", c)
	}
	code, _ := s.do(t, "POST", "/support/tickets", supRider2, "delivery_driver",
		map[string]any{"subject": "Order issue", "description": "customer absent", "order_id": 555})
	if code != http.StatusBadRequest {
		t.Errorf("someone else's order = %d, want 400", code)
	}
}

func TestSupportTicketDailyLimit(t *testing.T) {
	s := newSupportRig(t)
	for i := 0; i < maxTicketsPerRiderPerDay; i++ {
		code, _ := s.do(t, "POST", "/support/tickets", supRider1, "delivery_driver",
			map[string]any{"subject": "Issue " + string(rune('A'+i)), "description": "details"})
		if code != http.StatusCreated {
			t.Fatalf("ticket %d = %d, want 201", i+1, code)
		}
	}
	code, _ := s.do(t, "POST", "/support/tickets", supRider1, "delivery_driver",
		map[string]any{"subject": "One more", "description": "details"})
	if code != http.StatusTooManyRequests {
		t.Fatalf("over the daily limit = %d, want 429", code)
	}
	// The limit is per rider.
	if code, _ := s.do(t, "POST", "/support/tickets", supRider2, "delivery_driver",
		map[string]any{"subject": "Mine", "description": "details"}); code != http.StatusCreated {
		t.Fatalf("another rider = %d, want 201", code)
	}
}

func TestSupportContactOnlyReportsWhatIsConfigured(t *testing.T) {
	s := newSupportRig(t)
	t.Setenv("SUPPORT_PHONE", "")
	t.Setenv("SUPPORT_EMAIL", "")
	t.Setenv("EMERGENCY_PHONE", "")
	_, res := s.do(t, "GET", "/support/contact", supRider1, "delivery_driver", nil)
	d := dataOf(res)
	if d["support_phone"] != "" || d["support_email"] != "" || d["emergency_phone"] != "" {
		t.Fatalf("unconfigured contact must be empty, got %v", d)
	}

	t.Setenv("EMERGENCY_PHONE", " 112 ")
	_, res = s.do(t, "GET", "/support/contact", supRider1, "delivery_driver", nil)
	if dataOf(res)["emergency_phone"] != "112" {
		t.Fatalf("emergency phone = %v", dataOf(res)["emergency_phone"])
	}
}

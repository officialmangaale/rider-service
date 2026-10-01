package handler

// Rider support tickets against PRODUCTION'S REAL SCHEMA (schema only), with the
// real `users` and `delivery_orders` tables. Skipped unless TEST_REALSCHEMA_URL is
// set; refuses any non-local host. Needs migration 012 applied in that database.

import (
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
)

func realSupportRig(t *testing.T) *supportRig {
	t.Helper()
	raw := os.Getenv("TEST_REALSCHEMA_URL")
	if raw == "" {
		t.Skip("TEST_REALSCHEMA_URL not set")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatalf("refusing to run against a non-local database")
	}
	db, err := sql.Open("postgres", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

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
	admin := r.Group("/admin/support-tickets", middleware.RequireAdmin())
	admin.GET("", h.AdminListTickets)
	admin.PATCH("/:id", h.AdminUpdateTicket)
	return &supportRig{db: db, router: r}
}

func TestRealSchemaSupportTicketFlow(t *testing.T) {
	s := realSupportRig(t)

	riderPhone := "9" + strconv.FormatInt(time.Now().UnixNano()%1000000000, 10)
	for len(riderPhone) < 10 {
		riderPhone += "0"
	}
	var rider, admin string
	if err := s.db.QueryRow(`INSERT INTO users (first_name, last_name, phone, primary_role, status) VALUES ('Asha','Rider',$1,'delivery_driver','active') RETURNING id::text`, riderPhone).Scan(&rider); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`INSERT INTO users (first_name, last_name, primary_role, status) VALUES ('Ops','Admin','admin','active') RETURNING id::text`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	var rest int64
	if err := s.db.QueryRow(`INSERT INTO restaurants (name) VALUES ('Local Kitchen') RETURNING restaurant_id`).Scan(&rest); err != nil {
		t.Fatal(err)
	}
	// A real delivery this rider handled, so the order reference is accepted.
	var orderID int64
	if err := s.db.QueryRow(`INSERT INTO orders (restaurant_id, order_type, order_status) VALUES ($1,'DELIVERY','delivered') RETURNING order_id`, rest).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO delivery_orders (order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, drop_latitude, drop_longitude, delivery_status, assigned_rider_id, rider_user_id)
		VALUES ($1,$2,1,28.4,77.0,28.5,77.1,'delivered',$3,$3)`, orderID, rest, rider); err != nil {
		t.Fatalf("seed delivery order: %v", err)
	}

	code, res := s.do(t, "POST", "/support/tickets", rider, "delivery_driver",
		map[string]any{"subject": "COD mismatch", "description": "Collected 250 but app shows 200", "category": "payout", "order_id": orderID})
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, res)
	}
	id := int64(dataOf(res)["id"].(float64))

	_, res = s.do(t, "GET", "/support/tickets", rider, "delivery_driver", nil)
	if list := dataOf(res)["tickets"].([]any); len(list) != 1 {
		t.Fatalf("rider list = %d, want 1", len(list))
	}

	_, res = s.do(t, "GET", "/admin/support-tickets?status=open", admin, "admin", nil)
	row := dataOf(res)["tickets"].([]any)[0].(map[string]any)
	if row["rider_name"] != "Asha Rider" || row["rider_phone"] != riderPhone {
		t.Fatalf("admin row = %v", row)
	}

	code, _ = s.do(t, "PATCH", "/admin/support-tickets/"+itoa(id), admin, "admin", map[string]any{"status": "resolved", "admin_note": "Wallet corrected"})
	if code != http.StatusOK {
		t.Fatalf("resolve = %d", code)
	}
	var handledBy sql.NullString
	var resolvedAt sql.NullTime
	if err := s.db.QueryRow(`SELECT handled_by::text, resolved_at FROM rider_support_tickets WHERE id=$1`, id).Scan(&handledBy, &resolvedAt); err != nil {
		t.Fatal(err)
	}
	if handledBy.String != admin || !resolvedAt.Valid {
		t.Fatalf("handled_by=%q resolved_at=%v, want the admin and a timestamp", handledBy.String, resolvedAt)
	}
}

func itoa(n int64) string {
	b := [20]byte{}
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

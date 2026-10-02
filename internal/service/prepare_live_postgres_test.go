package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// Exercises the real database, dispatcher and authenticated socket. The SQL
// transition represents a committed restaurant update, not an HTTP placement
// or an AWS SQS delivery; those boundaries require deployed integration access.
func TestOnlineDispatchPrepareLiveOfferAndAssignment(t *testing.T) {
	db, s := onlineFixture(t)
	ctx := context.Background()
	router := gin.New()
	router.GET("/ws/rider", s.hub.HandleRiderWS("local-test-secret"))
	server := httptest.NewServer(router)
	defer server.Close()
	connect := func() *websocket.Conn {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": lcRider, "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte("local-test-secret"))
		if err != nil {
			t.Fatal(err)
		}
		header := http.Header{"Authorization": []string{"Bearer " + token}}
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/rider", header)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		deadline := time.Now().Add(time.Second)
		for s.hub.RiderConnectionCount(lcRider) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if s.hub.RiderConnectionCount(lcRider) == 0 {
			t.Fatal("socket not registered")
		}
		return conn
	}
	conn := connect()
	if _, err := db.Exec(`UPDATE orders SET order_status='pending' WHERE order_id=100`); err != nil {
		t.Fatal(err)
	}
	onlineEvent(t, s, 100)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM delivery_orders WHERE order_id=100`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("dispatch before prepare: %d %v", count, err)
	}
	if _, err := db.Exec(`UPDATE orders SET order_status='preparing' WHERE order_id=100`); err != nil {
		t.Fatal(err)
	}
	onlineEvent(t, s, 100)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var event struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	var offer struct {
		RequestID int       `json:"request_id"`
		OrderID   int       `json:"order_id"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(event.Data, &offer); err != nil {
		t.Fatal(err)
	}
	if event.Type != "DELIVERY_ORDER_REQUEST" || offer.OrderID != 100 || offer.RequestID == 0 || !offer.ExpiresAt.After(time.Now()) {
		t.Fatalf("invalid actionable event: %s %s", event.Type, event.Data)
	}
	request, _ := requestFor(t, db, 100, lcRider)
	if request != offer.RequestID {
		t.Fatal("socket targeted a different rider's offer")
	}
	onlineEvent(t, s, 100)
	if err := db.QueryRow(`SELECT count(*) FROM delivery_order_requests WHERE order_id=100 AND rider_id=$1`, lcRider).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate offer: %d %v", count, err)
	}
	conn.Close()
	connect()
	pending, err := s.GetPendingRequestPayloads(ctx, lcRider)
	if err != nil || len(pending) != 1 || pending[0]["request_id"] != request {
		t.Fatalf("reconnect lost offer: %v %v", pending, err)
	}
	accepted, err := s.AcceptRequest(ctx, request, lcRider)
	if err != nil || accepted == nil || accepted.OrderID != 100 {
		t.Fatalf("accept: %+v %v", accepted, err)
	}
	if _, err := s.AcceptRequest(ctx, request, lcRider); err != nil {
		t.Fatalf("duplicate accept: %v", err)
	}
	var rider, name, status, payment string
	if err := db.QueryRow(`SELECT assigned_rider_user_id,assigned_rider_name,order_status,payment_status FROM orders WHERE order_id=100`).Scan(&rider, &name, &status, &payment); err != nil {
		t.Fatal(err)
	}
	if rider != lcRider || name != "First" || status != "preparing" || payment != "pending" {
		t.Fatalf("restaurant projection: %q %q %q %q", rider, name, status, payment)
	}
	if pending, err := s.GetPendingRequestPayloads(ctx, lcRider); err != nil || len(pending) != 0 {
		t.Fatalf("accepted offer still pending: %v %v", pending, err)
	}
}

func TestOnlineDispatchDiagnosticsMatchSelection(t *testing.T) {
	cases := []struct{ name, sql, reason string }{
		{"eligible", `SELECT 1`, ""},
		{"disabled_account", `UPDATE users SET status='inactive'`, "rider_account_ineligible"},
		{"deleted_state", `UPDATE rider_locations SET is_deleted=true`, "rider_state_deleted"},
		{"invalid_location", `UPDATE rider_locations SET latitude=91`, "rider_location_invalid"},
		{"future_location", `UPDATE rider_locations SET last_updated_at=NOW()+INTERVAL '1 minute'`, "rider_location_in_future"},
		{"configured_freshness", `UPDATE online_delivery_dispatch_config SET location_max_age_seconds=30; UPDATE rider_locations SET last_updated_at=NOW()-INTERVAL '40 seconds'`, "rider_location_stale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, s := onlineFixture(t)
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			decision, err := s.deliveryRepo.RiderDecisionVector(ctx, lcRider, 28.4139, 77.0422, 5)
			if err != nil || decision.FirstFailure() != tc.reason {
				t.Fatalf("decision %+v err=%v want=%s", decision, err, tc.reason)
			}
			selected, err := s.deliveryRepo.FindNearestRiders(ctx, 28.4139, 77.0422, 5, 10)
			if err != nil {
				t.Fatal(err)
			}
			summary, err := s.deliveryRepo.GetRiderEligibilitySummary(ctx, 28.4139, 77.0422, 5)
			if err != nil || summary.RidersWithinRadius != len(selected) {
				t.Fatalf("funnel=%+v selected=%d err=%v", summary, len(selected), err)
			}
			if (len(selected) > 0) != (tc.reason == "") {
				t.Fatalf("unexpected selection: %+v", selected)
			}
		})
	}
}

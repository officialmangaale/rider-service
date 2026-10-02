package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
)

// Production-shaped dispatch journey.
//
// Every other PostgreSQL test here builds a small synthetic schema and applies
// migration 097 itself, so none of them can notice that production has not had
// 095/096/097 applied. This test runs the real service and repository against
// a scratch copy of the PRODUCTION schema (pg_dump --schema-only, no data) and
// shows both halves: what the deployed code does on the un-migrated schema, and
// the complete order -> offer -> socket -> accept journey once 095-097 are in.
//
// PROD_SHAPE_DATABASE_URL must name a loopback database called exactly
// "prod_shape", freshly loaded from that dump. The guards refuse anything else,
// so this cannot be pointed at a shared or production database. Skipped
// without it.
const (
	prodShapeEnv        = "PROD_SHAPE_DATABASE_URL"
	prodShapeRestaurant = 19
	prodShapeOtherShop  = 20
	prodShapeOrder      = 14659
	prodShapeOtherOrder = 14660
	prodShapeRider      = "756eacb9-c339-4c10-b8de-9000af95f917"
	prodShapeLat        = 29.437194
	prodShapeLng        = 78.715286
)

func openProdShape(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv(prodShapeEnv)
	if raw == "" {
		t.Skip(prodShapeEnv + " not set")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if h := u.Hostname(); h != "127.0.0.1" && h != "localhost" {
		t.Fatalf("refusing to run against non-loopback host %q", h)
	}
	if name := strings.TrimPrefix(u.Path, "/"); name != "prod_shape" {
		t.Fatalf("refusing to run against database %q", name)
	}
	db, err := sql.Open("postgres", raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var migrated bool
	var orders int
	if err := db.QueryRow(`SELECT to_regclass('public.online_delivery_dispatch_config') IS NOT NULL,
		(SELECT count(*) FROM orders)`).Scan(&migrated, &orders); err != nil {
		t.Fatal(err)
	}
	if migrated || orders != 0 {
		t.Fatal("scratch database must be freshly loaded from the schema-only production dump")
	}
	return db
}

func applyMigrations(t *testing.T, db *sql.DB, names ...string) {
	t.Helper()
	for _, name := range names {
		body, err := os.ReadFile("../../../restaurant-service/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
}

func seedProdShape(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
INSERT INTO restaurants(restaurant_id,name,latitude,longitude,street_address,city,state,postal_code)
  OVERRIDING SYSTEM VALUE VALUES
  (19,'iconic',29.437194,78.715286,'Pickup Rd','Ajimulla Nagar','Uttar Pradesh','246722'),
  (20,'other shop',29.437194,78.715286,'Pickup Rd','Ajimulla Nagar','Uttar Pradesh','246722');
INSERT INTO users(id,primary_role,status,first_name,last_name)
  VALUES ('` + prodShapeRider + `','delivery_driver','active','gursevak','gill');
-- Same shape as the live customer orders: every customer_web order carries
-- delivery_mode=restaurant_own_rider, and the restaurant has a My Riders roster.
INSERT INTO orders(order_id,restaurant_id,order_type,order_status,metadata)
  OVERRIDING SYSTEM VALUE VALUES
  (14659,19,'DELIVERY','pending','{"order_source":"customer_web","delivery_mode":"restaurant_own_rider"}'),
  (14660,20,'DELIVERY','pending','{"order_source":"customer_web","delivery_mode":"restaurant_own_rider"}');
INSERT INTO restaurant_riders(restaurant_id,rider_user_id,rider_name,status,is_active)
  VALUES (19,'bd84da69-3a63-4e75-a5cc-2c0be70a9be9','Own rider','active',true);
-- Gursevak, 0.01 km from the pickup, fresh GPS, online, available.
INSERT INTO rider_locations(rider_id,latitude,longitude,last_updated_at)
  VALUES ('` + prodShapeRider + `',29.4371941,78.7152858,NOW());
INSERT INTO rider_availability(rider_id,is_online,is_available,current_order_id)
  VALUES ('` + prodShapeRider + `',true,true,NULL);`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestProductionShapeMissingMigrationsThenFullJourney(t *testing.T) {
	db := openProdShape(t)
	seedProdShape(t, db)
	s := e2eService(db)
	t.Cleanup(func() { s.background.Wait() })
	ctx := context.Background()

	prepare := func(order int) {
		t.Helper()
		if _, err := db.Exec(`UPDATE orders SET order_status='preparing' WHERE order_id=$1`, order); err != nil {
			t.Fatal(err)
		}
	}
	event := func(order, restaurant int) error {
		e := confirmedOrderEvent(order)
		e.RestaurantID = restaurant
		e.Pickup.Latitude, e.Pickup.Longitude = prodShapeLat, prodShapeLng
		return s.ProcessOrderPlacedEvent(ctx, e)
	}
	count := func(query string, args ...interface{}) int {
		t.Helper()
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// 1. The deployed rider-service against the un-migrated production schema.
	prepare(prodShapeOrder)
	err := event(prodShapeOrder, prodShapeRestaurant)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("un-migrated schema must fail loudly, got %v", err)
	}
	t.Logf("un-migrated production schema: consumer error = %v", err)
	if n := count(`SELECT count(*) FROM delivery_orders`); n != 0 {
		t.Fatalf("a delivery projection exists without dispatch: %d", n)
	}
	if err := s.ReconcileFoodDispatches(ctx); err == nil {
		t.Fatal("recovery worker cannot succeed on the un-migrated schema")
	} else {
		t.Logf("un-migrated production schema: recovery worker error = %v", err)
	}

	// The boot-time preflight names every gap on the un-migrated schema.
	gaps, err := s.deliveryRepo.DispatchSchemaGaps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantGaps := []string{
		"078_offline_order_identity: orders.creation_source",
		"096_grocery_platform_delivery: delivery_orders.order_type",
		"096_grocery_platform_delivery: delivery_order_requests.order_type",
		"097_online_delivery_dispatch: online_delivery_dispatch_config",
		"097_online_delivery_dispatch: mangaale_dispatch_enabled()",
		"097_online_delivery_dispatch: mangaale_online_delivery()",
	}
	if strings.Join(gaps, "|") != strings.Join(wantGaps, "|") {
		t.Fatalf("schema gaps = %v, want %v", gaps, wantGaps)
	}

	// 2. Apply the missing migrations in their documented order. 095-097 alone
	// are not enough: the rider consumer's policy predicate also reads
	// orders.creation_source, which only migration 078 adds and production
	// never received.
	applyMigrations(t, db,
		"095_grocery_delivery_completion.sql",
		"096_grocery_platform_delivery.sql",
		"097_online_delivery_dispatch.sql")
	if err := event(prodShapeOrder, prodShapeRestaurant); err == nil || !strings.Contains(err.Error(), "creation_source") {
		t.Fatalf("095-097 without 078 must still fail on creation_source, got %v", err)
	}
	if gaps, err := s.deliveryRepo.DispatchSchemaGaps(ctx); err != nil || len(gaps) != 1 || !strings.Contains(gaps[0], "078_offline_order_identity") {
		t.Fatalf("only 078 should remain missing, got %v %v", gaps, err)
	}
	applyMigrations(t, db, "078_offline_order_identity.sql")
	if gaps, err := s.deliveryRepo.DispatchSchemaGaps(ctx); err != nil || len(gaps) != 0 {
		t.Fatalf("schema still incomplete after 078/095/096/097: %v %v", gaps, err)
	}
	// Dispatch is still OFF: 097 ships disabled and never enables anything
	// by itself.
	if err := event(prodShapeOrder, prodShapeRestaurant); err != nil {
		t.Fatal(err)
	}
	reason, err := s.deliveryRepo.FoodDispatchBlockReason(ctx, prodShapeOrder)
	if err != nil || reason != "dispatch_disabled_or_restaurant_not_enabled" {
		t.Fatalf("flag-off reason = %q, %v", reason, err)
	}
	if n := count(`SELECT count(*) FROM delivery_orders`); n != 0 {
		t.Fatalf("dispatch ran while disabled: %d", n)
	}

	// 3. Intentional, allowlisted rollout: restaurant 19 only.
	if _, err := db.Exec(`UPDATE online_delivery_dispatch_config
		SET restaurant_ids = ARRAY[19]::bigint[], enabled = true WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	prepare(prodShapeOtherOrder)
	if err := event(prodShapeOtherOrder, prodShapeOtherShop); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT count(*) FROM delivery_orders WHERE order_id=$1`, prodShapeOtherOrder); n != 0 {
		t.Fatal("a restaurant outside the allowlist was dispatched")
	}

	// 4. Restaurant 19: authenticated rider socket, then the Prepare event.
	router := gin.New()
	router.GET("/ws/rider", s.hub.HandleRiderWS("local-test-secret"))
	server := httptest.NewServer(router)
	defer server.Close()
	connect := func() *websocket.Conn {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": prodShapeRider, "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte("local-test-secret"))
		if err != nil {
			t.Fatal(err)
		}
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/rider",
			http.Header{"Authorization": []string{"Bearer " + token}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		for deadline := time.Now().Add(time.Second); s.hub.RiderConnectionCount(prodShapeRider) == 0 && time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
		}
		if s.hub.RiderConnectionCount(prodShapeRider) == 0 {
			t.Fatal("socket not registered")
		}
		return conn
	}
	conn := connect()
	if err := event(prodShapeOrder, prodShapeRestaurant); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var frame struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("no actionable offer reached the rider socket: %v", err)
	}
	var offer struct {
		RequestID int       `json:"request_id"`
		OrderID   int       `json:"order_id"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(frame.Data, &offer); err != nil {
		t.Fatal(err)
	}
	if frame.Type != "DELIVERY_ORDER_REQUEST" || offer.OrderID != prodShapeOrder || offer.RequestID <= 0 || !offer.ExpiresAt.After(time.Now()) {
		t.Fatalf("not an actionable offer: %s %s", frame.Type, frame.Data)
	}
	t.Logf("socket offer: request_id=%d order_id=%d expires_in=%s", offer.RequestID, offer.OrderID, time.Until(offer.ExpiresAt).Round(time.Second))
	request, status := requestFor(t, db, prodShapeOrder, prodShapeRider)
	if request != offer.RequestID || status != "pending" {
		t.Fatalf("persisted offer %d/%s does not match socket offer %d", request, status, offer.RequestID)
	}

	// Duplicate event: idempotent. Reconnect: the pending API still has it.
	if err := event(prodShapeOrder, prodShapeRestaurant); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT count(*) FROM delivery_order_requests WHERE order_id=$1 AND rider_id=$2`, prodShapeOrder, prodShapeRider); n != 1 {
		t.Fatalf("duplicate event created %d offers", n)
	}
	conn.Close()
	connect()
	pending, err := s.GetPendingRequestPayloads(ctx, prodShapeRider)
	if err != nil || len(pending) != 1 || pending[0]["request_id"] != request {
		t.Fatalf("reconnect lost the offer: %v %v", pending, err)
	}

	// 5. Accept, then the restaurant-side projection.
	if _, err := s.AcceptRequest(ctx, request, prodShapeRider); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := s.AcceptRequest(ctx, request, prodShapeRider); err != nil {
		t.Fatalf("repeated accept must be idempotent: %v", err)
	}
	var rider, name, orderStatus, payment, delivery string
	if err := db.QueryRow(`SELECT COALESCE(assigned_rider_user_id,''),COALESCE(assigned_rider_name,''),
		order_status::text,COALESCE(payment_status::text,''),COALESCE(delivery_status::text,'')
		FROM orders WHERE order_id=$1`, prodShapeOrder).Scan(&rider, &name, &orderStatus, &payment, &delivery); err != nil {
		t.Fatal(err)
	}
	t.Logf("restaurant projection: rider=%s name=%q order_status=%s payment=%s delivery=%s", rider, name, orderStatus, payment, delivery)
	if rider != prodShapeRider || !strings.Contains(strings.ToLower(name), "gursevak") {
		t.Fatalf("restaurant did not receive the rider details: %q %q", rider, name)
	}
	if orderStatus != "preparing" {
		t.Fatalf("assignment changed the food preparation status to %q", orderStatus)
	}
	if pending, err := s.GetPendingRequestPayloads(ctx, prodShapeRider); err != nil || len(pending) != 0 {
		t.Fatalf("accepted offer still pending: %v %v", pending, err)
	}
	if n := count(`SELECT count(*) FROM rider_availability WHERE rider_id=$1 AND current_order_id=$2`, prodShapeRider, prodShapeOrder); n != 1 {
		t.Fatal("rider was not made busy with the accepted order")
	}
}

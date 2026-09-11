package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// The delivery lifecycle after acceptance, on real PostgreSQL
// (TEST_DATABASE_URL; skipped without it), against a fake restaurant-service
// that enforces the real contract (restaurant-service
// controller/internal_order.go + services/order_status_contract.go):
//   - assign-rider records the rider on `orders`, idempotently;
//   - delivery-status is accepted only from the recorded rider;
//   - a rider may move the order only ready → out_for_delivery → delivered.
//
// The client is built with a trailing-slash base URL, exactly as production
// was configured, so these tests also pin the "//internal" 404 regression.

const (
	lcRider      = "c6b46748-0000-4000-8000-000000000001"
	lcOtherRider = "c6b46748-0000-4000-8000-000000000002"
	lcToken      = "test-internal-token"
)

type fakeRestaurant struct {
	t  *testing.T
	db *sql.DB

	mu            sync.Mutex
	assignCalls   int
	statusCalls   int
	failAssign    int // answer this many assign-rider calls with 500
	refuseStatus  int // answer delivery-status with this code (0 = follow contract)
	refuseMessage string
}

var internalPath = regexp.MustCompile(`^/internal/orders/(\d+)/(assign-rider|delivery-status)$`)

func (f *fakeRestaurant) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, msg string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": msg})
	}
	if r.Header.Get("X-Internal-Service-Token") != lcToken {
		reply(http.StatusUnauthorized, "invalid service token")
		return
	}
	m := internalPath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		reply(http.StatusNotFound, "404 page not found")
		return
	}
	orderID, _ := strconv.Atoi(m[1])
	f.mu.Lock()
	defer f.mu.Unlock()

	if m[2] == "assign-rider" {
		f.assignCalls++
		if f.failAssign > 0 {
			f.failAssign--
			reply(http.StatusInternalServerError, "failed to assign rider")
			return
		}
		var body struct {
			RiderID   string `json:"rider_id"`
			RiderName string `json:"rider_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		res, err := f.db.Exec(`UPDATE orders SET assigned_rider_user_id = $1, delivery_partner_id = $1, delivery_status = 'rider_assigned'
			WHERE order_id = $2 AND COALESCE(assigned_rider_user_id, '') IN ('', $1)`, body.RiderID, orderID)
		if n, _ := res.RowsAffected(); err != nil || n != 1 {
			reply(http.StatusConflict, "order already assigned to a different rider")
			return
		}
		reply(http.StatusOK, "rider assigned successfully")
		return
	}

	f.statusCalls++
	if f.refuseStatus != 0 {
		reply(f.refuseStatus, f.refuseMessage)
		return
	}
	var body struct {
		RiderUserID    string `json:"rider_user_id"`
		DeliveryStatus string `json:"delivery_status"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var orderStatus, recorded string
	if err := f.db.QueryRow(`SELECT order_status, COALESCE(assigned_rider_user_id, '') FROM orders WHERE order_id = $1`, orderID).
		Scan(&orderStatus, &recorded); err != nil {
		reply(http.StatusNotFound, "order not found")
		return
	}
	if recorded != body.RiderUserID {
		reply(http.StatusForbidden, "rider is not assigned to this order")
		return
	}
	target := "out_for_delivery"
	if body.DeliveryStatus == "delivered" {
		target = "delivered"
	}
	allowed := orderStatus == target ||
		(orderStatus == "ready" && target == "out_for_delivery") ||
		(orderStatus == "out_for_delivery" && target == "delivered")
	if !allowed {
		reply(http.StatusBadRequest, fmt.Sprintf("rider is not allowed to transition order from %s to %s", orderStatus, target))
		return
	}
	deliveryStatus := body.DeliveryStatus
	if deliveryStatus == "on_the_way" {
		deliveryStatus = "out_for_delivery"
	}
	if _, err := f.db.Exec(`UPDATE orders SET order_status = $1, delivery_status = $2 WHERE order_id = $3`, target, deliveryStatus, orderID); err != nil {
		reply(http.StatusInternalServerError, "failed to update status")
		return
	}
	reply(http.StatusOK, "delivery status updated")
}

func (f *fakeRestaurant) calls() (assign, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assignCalls, f.statusCalls
}

// lcCapture collects trace lines behind a lock the test also reads under,
// since acceptance-time sync emits from its own goroutine.
type lcCapture struct {
	mu    sync.Mutex
	lines []string
}

func (c *lcCapture) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.lines, "\n")
}

type lifecycle struct {
	db   *sql.DB
	svc  *DeliveryService
	fake *fakeRestaurant
	log  *lcCapture
}

func newLifecycle(t *testing.T) *lifecycle {
	t.Helper()
	db := testpg.Open(t)
	if _, err := db.Exec(testpg.LifecycleSchema); err != nil {
		t.Fatalf("lifecycle schema: %v", err)
	}
	for _, r := range []struct{ id, first string }{{lcRider, "Aman"}, {lcOtherRider, "Other"}} {
		if _, err := db.Exec(`INSERT INTO users (id, first_name, phone, primary_role, vehicle_type) VALUES ($1, $2, '+910000000001', 'delivery_driver', 'bike')`, r.id, r.first); err != nil {
			t.Fatalf("seed rider: %v", err)
		}
	}
	capture := &lcCapture{}
	restore := dispatchtrace.SetOutput(func(format string, args ...interface{}) {
		capture.mu.Lock()
		defer capture.mu.Unlock()
		capture.lines = append(capture.lines, fmt.Sprintf(format, args...))
	})
	t.Cleanup(restore)

	delays := assignmentRetryDelays
	assignmentRetryDelays = []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	t.Cleanup(func() { assignmentRetryDelays = delays })

	fake := &fakeRestaurant{t: t, db: db}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	svc := NewDeliveryService(
		repository.NewDeliveryRepository(db),
		repository.NewRiderRepository(db),
		ws.NewHub(),
		client.NewRestaurantClient(server.URL+"/", lcToken), // trailing slash, as in production
		5.0, 5, 30,
		nil,
	)
	// Registered last, so it runs first: background syncs finish before the
	// log sink, retry delays and server are torn down.
	t.Cleanup(svc.background.Wait)
	return &lifecycle{db: db, svc: svc, fake: fake, log: capture}
}

// seedAssigned creates an order the rider has already accepted. riderOnOrder
// says whether restaurant-service's `orders` row already records the rider.
func (l *lifecycle) seedAssigned(t *testing.T, orderID int, orderStatus string, riderOnOrder bool, paymentMode string) {
	t.Helper()
	recorded := sql.NullString{}
	if riderOnOrder {
		recorded = sql.NullString{String: lcRider, Valid: true}
	}
	if _, err := l.db.Exec(`INSERT INTO orders (order_id, restaurant_id, order_status, assigned_rider_user_id) VALUES ($1, 27, $2, $3)`,
		orderID, orderStatus, recorded); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if _, err := l.db.Exec(`INSERT INTO delivery_orders (order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, pickup_address,
			drop_latitude, drop_longitude, drop_address, amount, payment_mode, delivery_status, assigned_rider_id, rider_user_id, assigned_at,
			restaurant_name, restaurant_phone, customer_name, customer_phone)
		VALUES ($1, 27, 5, 28.41, 77.04, 'pickup', 28.43, 77.05, 'drop', 250, $2, 'rider_assigned', $3, $3, NOW(),
			'Test Kitchen', '+910000000002', 'Customer', '+910000000003')`, orderID, paymentMode, lcRider); err != nil {
		t.Fatalf("seed delivery order: %v", err)
	}
	if _, err := l.db.Exec(`INSERT INTO rider_availability (rider_id, is_online, is_available, current_order_id) VALUES ($1, true, false, $2)`,
		lcRider, orderID); err != nil {
		t.Fatalf("seed availability: %v", err)
	}
}

func (l *lifecycle) deliveryStatus(t *testing.T, orderID int) string {
	t.Helper()
	var s string
	if err := l.db.QueryRow(`SELECT delivery_status FROM delivery_orders WHERE order_id = $1`, orderID).Scan(&s); err != nil {
		t.Fatalf("delivery status: %v", err)
	}
	return s
}

func (l *lifecycle) orderRow(t *testing.T, orderID int) (status, rider string) {
	t.Helper()
	if err := l.db.QueryRow(`SELECT order_status, COALESCE(assigned_rider_user_id, '') FROM orders WHERE order_id = $1`, orderID).
		Scan(&status, &rider); err != nil {
		t.Fatalf("order row: %v", err)
	}
	return status, rider
}

func statusCode(err error) string {
	var se *DeliveryStatusError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Accepting records the rider on the customer's order: this is what makes
// new_user_app stop showing "Finding a delivery partner".
func TestAcceptedRiderIsRecordedOnTheCustomerOrder(t *testing.T) {
	l := newLifecycle(t)
	e2eSeedRider(t, l.db, lcRider, 0.5)
	e2eSeedRider(t, l.db, lcOtherRider, 1.0)
	if _, err := l.db.Exec(`INSERT INTO orders (order_id, restaurant_id, order_status) VALUES (13356, 27, 'confirmed')`); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	ctx := context.Background()
	if err := l.svc.ProcessOrderPlacedEvent(ctx, confirmedOrderEvent(13356)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}
	reqID, _ := requestFor(t, l.db, 13356, lcRider)
	otherReq, _ := requestFor(t, l.db, 13356, lcOtherRider)
	if _, err := l.svc.AcceptRequest(ctx, reqID, lcRider); err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}

	waitFor(t, "rider recorded on orders", func() bool {
		_, rider := l.orderRow(t, 13356)
		return rider == lcRider
	})
	waitFor(t, "assignment synced event", func() bool {
		return strings.Contains(l.log.text(), "event=restaurant.assignment.synced")
	})
	if _, status := requestFor(t, l.db, 13356, lcOtherRider); status != models.RequestStatusCancelled {
		t.Fatalf("other rider's offer is %q, want cancelled", status)
	}
	if _, err := l.svc.AcceptRequest(ctx, otherReq, lcOtherRider); err == nil {
		t.Fatal("the other rider must not be able to accept")
	}
	log := l.log.text()
	if !strings.Contains(log, "trigger=accept") || !strings.Contains(log, "has_name=true") {
		t.Fatalf("expected an accept-triggered sync with the rider's name, got:\n%s", log)
	}
}

// The acceptance-time sync keeps retrying server errors.
func TestAcceptanceSyncRetriesAServerError(t *testing.T) {
	l := newLifecycle(t)
	l.fake.failAssign = 2
	l.seedAssigned(t, 13370, "confirmed", false, "online")

	l.svc.syncRiderAssignmentAsync(13370, lcRider, time.Now())

	waitFor(t, "rider recorded after retries", func() bool {
		_, rider := l.orderRow(t, 13370)
		return rider == lcRider
	})
	if assign, _ := l.fake.calls(); assign != 3 {
		t.Fatalf("assign-rider calls = %d, want 3 (two 500s, then success)", assign)
	}
}

// The full rider sequence, with the kitchen releasing the order in between.
func TestDeliveryLifecycleValidSequence(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13380, "preparing", true, "cash")
	ctx := context.Background()

	if err := l.svc.UpdateDeliveryStatus(ctx, 13380, lcRider, models.DeliveryStatusRiderArrivedRestaurant, false, ""); err != nil {
		t.Fatalf("reached restaurant: %v", err)
	}
	// Kitchen marks the order ready (owner app / KDS / prep timer).
	if _, err := l.db.Exec(`UPDATE orders SET order_status = 'ready' WHERE order_id = 13380`); err != nil {
		t.Fatal(err)
	}
	if err := l.svc.UpdateDeliveryStatus(ctx, 13380, lcRider, models.DeliveryStatusPickedUp, false, ""); err != nil {
		t.Fatalf("picked up: %v", err)
	}
	if s, _ := l.orderRow(t, 13380); s != "out_for_delivery" {
		t.Fatalf("canonical order after pickup = %q, want out_for_delivery", s)
	}
	if err := l.svc.UpdateDeliveryStatus(ctx, 13380, lcRider, models.DeliveryStatusOnTheWay, false, ""); err != nil {
		t.Fatalf("on the way: %v", err)
	}
	if err := l.svc.UpdateDeliveryStatus(ctx, 13380, lcRider, models.DeliveryStatusDelivered, false, ""); statusCode(err) != ErrCodeCashNotConfirmed {
		t.Fatalf("delivered without cash confirmation: %v, want %s", err, ErrCodeCashNotConfirmed)
	}
	if err := l.svc.UpdateDeliveryStatus(ctx, 13380, lcRider, models.DeliveryStatusDelivered, true, ""); err != nil {
		t.Fatalf("delivered: %v", err)
	}

	if s := l.deliveryStatus(t, 13380); s != models.DeliveryStatusDelivered {
		t.Fatalf("delivery status %q", s)
	}
	if s, _ := l.orderRow(t, 13380); s != "delivered" {
		t.Fatalf("canonical order %q, want delivered", s)
	}
	var current sql.NullInt64
	var available bool
	if err := l.db.QueryRow(`SELECT current_order_id, is_available FROM rider_availability WHERE rider_id = $1`, lcRider).Scan(&current, &available); err != nil {
		t.Fatal(err)
	}
	if current.Valid || !available {
		t.Fatalf("rider must be free after delivery: current_order_id=%v available=%v", current, available)
	}
	var steps int
	if err := l.db.QueryRow(`SELECT COUNT(*) FROM delivery_status_history WHERE order_id = 13380`).Scan(&steps); err != nil || steps != 4 {
		t.Fatalf("history rows = %d (%v), want 4", steps, err)
	}
	log := l.log.text()
	for _, want := range []string{
		"event=delivery.status.updated from_status=rider_assigned order_id=13380 result=ok",
		"to_status=delivered",
		"reason_code=cash_not_confirmed",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("missing %q in:\n%s", want, log)
		}
	}
}

// Pickup before the kitchen releases the order is refused with a clear code
// and without calling restaurant-service; once ready, it succeeds.
func TestPickupBeforeKitchenReadyIsRefusedClearly(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13381, "preparing", true, "online")
	ctx := context.Background()
	if err := l.svc.UpdateDeliveryStatus(ctx, 13381, lcRider, models.DeliveryStatusRiderArrivedRestaurant, false, ""); err != nil {
		t.Fatal(err)
	}

	err := l.svc.UpdateDeliveryStatus(ctx, 13381, lcRider, models.DeliveryStatusPickedUp, false, "")
	if statusCode(err) != ErrCodeOrderNotReady {
		t.Fatalf("pickup while preparing: %v, want %s", err, ErrCodeOrderNotReady)
	}
	if _, status := l.fake.calls(); status != 0 {
		t.Fatalf("restaurant-service was called %d times for a pickup that cannot succeed", status)
	}
	if s := l.deliveryStatus(t, 13381); s != models.DeliveryStatusRiderArrivedRestaurant {
		t.Fatalf("delivery status moved to %q", s)
	}
	if !strings.Contains(l.log.text(), "reason_code=order_not_ready requested_status=picked_up restaurant_status=preparing") {
		t.Fatalf("missing explained rejection:\n%s", l.log.text())
	}

	if _, err := l.db.Exec(`UPDATE orders SET order_status = 'ready' WHERE order_id = 13381`); err != nil {
		t.Fatal(err)
	}
	if err := l.svc.UpdateDeliveryStatus(ctx, 13381, lcRider, models.DeliveryStatusPickedUp, false, ""); err != nil {
		t.Fatalf("pickup once ready: %v", err)
	}
}

// Order 13356's state in production: rider-service assigned, restaurant-service
// never told. The next canonical step records the rider first, then succeeds.
func TestStatusUpdateRecordsAMissingAssignmentFirst(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13356, "ready", false, "online")
	ctx := context.Background()
	if err := l.svc.UpdateDeliveryStatus(ctx, 13356, lcRider, models.DeliveryStatusRiderArrivedRestaurant, false, ""); err != nil {
		t.Fatal(err)
	}
	// Reaching the restaurant already repairs it, best effort.
	if _, rider := l.orderRow(t, 13356); rider != lcRider {
		t.Fatalf("reached-restaurant should record the rider; orders has %q", rider)
	}
	if _, err := l.db.Exec(`UPDATE orders SET assigned_rider_user_id = NULL, delivery_partner_id = NULL WHERE order_id = 13356`); err != nil {
		t.Fatal(err)
	}

	if err := l.svc.UpdateDeliveryStatus(ctx, 13356, lcRider, models.DeliveryStatusPickedUp, false, ""); err != nil {
		t.Fatalf("pickup with missing assignment: %v", err)
	}
	status, rider := l.orderRow(t, 13356)
	if rider != lcRider || status != "out_for_delivery" {
		t.Fatalf("orders after pickup: status=%q rider=%q", status, rider)
	}
	if !strings.Contains(l.log.text(), "trigger=status_update") {
		t.Fatalf("expected a status_update-triggered sync:\n%s", l.log.text())
	}
}

// When the rider cannot be recorded, the pickup is refused with a retryable
// code and the delivery does not advance.
func TestStatusUpdateFailsClosedWhenTheAssignmentCannotBeRecorded(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13382, "ready", false, "online")
	l.fake.failAssign = 100
	if _, err := l.db.Exec(`UPDATE delivery_orders SET delivery_status = 'rider_arrived_restaurant' WHERE order_id = 13382`); err != nil {
		t.Fatal(err)
	}
	err := l.svc.UpdateDeliveryStatus(context.Background(), 13382, lcRider, models.DeliveryStatusPickedUp, false, "")
	if statusCode(err) != ErrCodeRestaurantSync {
		t.Fatalf("got %v, want %s", err, ErrCodeRestaurantSync)
	}
	if s := l.deliveryStatus(t, 13382); s != models.DeliveryStatusRiderArrivedRestaurant {
		t.Fatalf("delivery advanced to %q", s)
	}
	if !strings.Contains(l.log.text(), "event=restaurant.assignment.sync_failed error=failed_to_assign_rider http_status=500") {
		t.Fatalf("missing sync failure with restaurant-service's reason:\n%s", l.log.text())
	}
}

// restaurant-service's own refusal reason reaches the error and the log.
func TestRestaurantRefusalCarriesItsReason(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13383, "ready", true, "online")
	l.fake.refuseStatus = http.StatusConflict
	l.fake.refuseMessage = "order status changed concurrently"
	if _, err := l.db.Exec(`UPDATE delivery_orders SET delivery_status = 'rider_arrived_restaurant' WHERE order_id = 13383`); err != nil {
		t.Fatal(err)
	}
	err := l.svc.UpdateDeliveryStatus(context.Background(), 13383, lcRider, models.DeliveryStatusPickedUp, false, "")
	if statusCode(err) != ErrCodeRestaurantSync || !strings.Contains(err.Error(), "status 409") ||
		!strings.Contains(err.Error(), "order status changed concurrently") {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(l.log.text(), "http_status=409 order_id=13383 reason_code=restaurant_rejected") {
		t.Fatalf("missing rejection log:\n%s", l.log.text())
	}
}

func TestRepeatingTheCurrentStepIsIdempotent(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13384, "ready", true, "online")
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := l.svc.UpdateDeliveryStatus(ctx, 13384, lcRider, models.DeliveryStatusRiderArrivedRestaurant, false, ""); err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	var steps int
	if err := l.db.QueryRow(`SELECT COUNT(*) FROM delivery_status_history WHERE order_id = 13384`).Scan(&steps); err != nil || steps != 1 {
		t.Fatalf("history rows = %d (%v), want 1", steps, err)
	}
	if !strings.Contains(l.log.text(), "reason_code=already_in_status result=unchanged") {
		t.Fatalf("missing unchanged event:\n%s", l.log.text())
	}
}

func TestSkippingAStepIsRejected(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13385, "ready", true, "online")
	err := l.svc.UpdateDeliveryStatus(context.Background(), 13385, lcRider, models.DeliveryStatusPickedUp, false, "")
	if statusCode(err) != ErrCodeInvalidTransition {
		t.Fatalf("got %v, want %s", err, ErrCodeInvalidTransition)
	}
	if err.Error() != "invalid transition from 'rider_assigned' to 'picked_up'" {
		t.Fatalf("message changed: %q", err.Error())
	}
	if !strings.Contains(l.log.text(), "expected_status=rider_arrived_restaurant") {
		t.Fatalf("missing expected status:\n%s", l.log.text())
	}
}

func TestOnlyTheAssignedRiderCanUpdate(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13386, "ready", true, "online")
	err := l.svc.UpdateDeliveryStatus(context.Background(), 13386, lcOtherRider, models.DeliveryStatusRiderArrivedRestaurant, false, "")
	if statusCode(err) != ErrCodeNotAssignedRider {
		t.Fatalf("got %v, want %s", err, ErrCodeNotAssignedRider)
	}
	if s := l.deliveryStatus(t, 13386); s != models.DeliveryStatusRiderAssigned {
		t.Fatalf("status moved to %q", s)
	}
}

// The rider's active delivery carries both stops, contacts, the kitchen's
// state and the next step, and the log line carries none of the values.
func TestActiveDeliveryCarriesNavigationContactsAndNextStep(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13387, "preparing", true, "online")
	orders := NewOrderService(nil, repository.NewDeliveryRepository(l.db), nil, nil, nil, nil, nil)

	active, err := orders.GetActiveOrder(context.Background(), lcRider)
	if err != nil {
		t.Fatalf("GetActiveOrder: %v", err)
	}
	if active.PickupAddress != "pickup" || active.DropAddress != "drop" ||
		active.PickupLatitude == nil || active.DropLongitude == nil {
		t.Fatalf("navigation data missing: %+v", active)
	}
	if active.RestaurantPhone == "" || active.CustomerName != "Customer" || active.CustomerPhone == "" {
		t.Fatalf("contacts missing: %+v", active)
	}
	if active.RestaurantOrderStatus != "preparing" || active.PickupReady == nil || *active.PickupReady {
		t.Fatalf("kitchen state: status=%q ready=%v", active.RestaurantOrderStatus, active.PickupReady)
	}
	if active.NextDeliveryStatus != models.DeliveryStatusRiderArrivedRestaurant {
		t.Fatalf("next step %q", active.NextDeliveryStatus)
	}
	log := l.log.text()
	if !strings.Contains(log, "event=rider.active_delivery.returned") || !strings.Contains(log, "navigation_data=complete") {
		t.Fatalf("missing active delivery event:\n%s", log)
	}
	for _, leaked := range []string{"+91", "28.41", "77.04", "pickup_address", "Customer"} {
		if strings.Contains(log, leaked) {
			t.Fatalf("log leaked %q:\n%s", leaked, log)
		}
	}
}

// Production, order 13356: the owner completed the order while the rider was
// at the restaurant. The rider kept current_order_id=13356 and was excluded
// from every later offer. Each way the owner can end an order releases them.
func TestOrderClosedByTheRestaurantReleasesTheRider(t *testing.T) {
	for i, closed := range []string{"completed", "cancelled", "rejected"} {
		t.Run(closed, func(t *testing.T) {
			l := newLifecycle(t)
			orderID := 13356 + i
			l.seedAssigned(t, orderID, closed, true, "cash")
			if _, err := l.db.Exec(`UPDATE delivery_orders SET delivery_status = 'rider_arrived_restaurant' WHERE order_id = $1`, orderID); err != nil {
				t.Fatal(err)
			}
			if _, err := l.db.Exec(`INSERT INTO rider_locations (rider_id, latitude, longitude) VALUES ($1, 28.41, 77.04)`, lcRider); err != nil {
				t.Fatal(err)
			}
			repo := repository.NewDeliveryRepository(l.db)
			ctx := context.Background()
			if riders, _ := repo.FindNearestRiders(ctx, 28.41, 77.04, 5, 5); len(riders) != 0 {
				t.Fatalf("a busy rider must not be offerable: %v", riders)
			}

			released, err := l.svc.ReleaseClosedDeliveries(ctx)
			if err != nil || released != 1 {
				t.Fatalf("released %d, %v; want 1", released, err)
			}
			if s := l.deliveryStatus(t, orderID); s != models.DeliveryStatusCancelled {
				t.Fatalf("delivery status %q, want cancelled", s)
			}
			var current sql.NullInt64
			if err := l.db.QueryRow(`SELECT current_order_id FROM rider_availability WHERE rider_id = $1`, lcRider).Scan(&current); err != nil || current.Valid {
				t.Fatalf("rider still busy: %v %v", current, err)
			}
			if riders, err := repo.FindNearestRiders(ctx, 28.41, 77.04, 5, 5); err != nil || len(riders) != 1 {
				t.Fatalf("released rider must be offerable again: %v %v", riders, err)
			}
			var reason string
			if err := l.db.QueryRow(`SELECT metadata->>'reason' FROM delivery_status_history WHERE order_id = $1 AND to_status = 'cancelled'`, orderID).Scan(&reason); err != nil || reason != "restaurant_closed_order" {
				t.Fatalf("history reason %q, %v", reason, err)
			}
			var payouts int
			if err := l.db.QueryRow(`SELECT COUNT(*) FROM rider_earnings WHERE order_id = $1`, orderID).Scan(&payouts); err != nil || payouts != 0 {
				t.Fatalf("a released delivery must not pay out: %d %v", payouts, err)
			}
			if again, _ := l.svc.ReleaseClosedDeliveries(ctx); again != 0 {
				t.Fatalf("second sweep released %d", again)
			}
			if !strings.Contains(l.log.text(), "event=delivery.released") || !strings.Contains(l.log.text(), "restaurant_status="+closed) {
				t.Fatalf("missing release event:\n%s", l.log.text())
			}
		})
	}
}

// The rider taps a step on an order the owner already closed: a clear code,
// and the rider is freed at once.
func TestStatusUpdateOnAClosedOrderReleasesAndSaysSo(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13390, "completed", true, "online")
	if _, err := l.db.Exec(`UPDATE delivery_orders SET delivery_status = 'rider_arrived_restaurant' WHERE order_id = 13390`); err != nil {
		t.Fatal(err)
	}
	err := l.svc.UpdateDeliveryStatus(context.Background(), 13390, lcRider, models.DeliveryStatusPickedUp, false, "")
	if statusCode(err) != ErrCodeOrderClosed {
		t.Fatalf("got %v, want %s", err, ErrCodeOrderClosed)
	}
	if _, status := l.fake.calls(); status != 0 {
		t.Fatal("restaurant-service must not be asked to move a closed order")
	}
	if s := l.deliveryStatus(t, 13390); s != models.DeliveryStatusCancelled {
		t.Fatalf("delivery status %q", s)
	}
	if _, err := repository.NewDeliveryRepository(l.db).GetActiveOrderForRider(context.Background(), lcRider); err != sql.ErrNoRows {
		t.Fatalf("closed order must not stay the rider's active delivery: %v", err)
	}
}

// Before the sweep runs, the app is not shown an order it can no longer act on.
func TestActiveOrderHidesAnOrderTheRestaurantClosed(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13391, "completed", true, "online")
	orders := NewOrderService(nil, repository.NewDeliveryRepository(l.db), nil, nil, nil, nil, nil)
	if _, err := orders.GetActiveOrder(context.Background(), lcRider); err != sql.ErrNoRows {
		t.Fatalf("got %v, want sql.ErrNoRows", err)
	}
}

// An owner marking the order delivered is not a closure: the rider can still
// confirm the delivery, and the sweep leaves it alone.
func TestOwnerMarkedDeliveredStillLetsTheRiderFinish(t *testing.T) {
	l := newLifecycle(t)
	l.seedAssigned(t, 13392, "delivered", true, "online")
	if _, err := l.db.Exec(`UPDATE delivery_orders SET delivery_status = 'on_the_way' WHERE order_id = 13392`); err != nil {
		t.Fatal(err)
	}
	if released, _ := l.svc.ReleaseClosedDeliveries(context.Background()); released != 0 {
		t.Fatalf("sweep released %d", released)
	}
	if err := l.svc.UpdateDeliveryStatus(context.Background(), 13392, lcRider, models.DeliveryStatusDelivered, false, ""); err != nil {
		t.Fatalf("delivered: %v", err)
	}
	if s := l.deliveryStatus(t, 13392); s != models.DeliveryStatusDelivered {
		t.Fatalf("delivery status %q", s)
	}
}

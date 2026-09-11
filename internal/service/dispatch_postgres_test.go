package service

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// End to end on real PostgreSQL (TEST_DATABASE_URL; skipped without it):
// a confirmed order's ORDER_PLACED event → rider search → offers persisted and
// published → the rider app's pending-requests read → accept → exclusive
// assignment. Before the FindNearestRiders fix, the search step failed on
// every call and none of the later steps ever ran.

const (
	e2ePickupLat = 28.4139
	e2ePickupLng = 77.0422
	e2eKm        = 1.0 / 111.0
)

func e2eSeedRider(t *testing.T, db *sql.DB, id string, kmFromPickup float64) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO rider_availability (rider_id, is_online, is_available) VALUES ($1, true, true)`, id); err != nil {
		t.Fatalf("seed availability: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO rider_locations (rider_id, latitude, longitude) VALUES ($1, $2, $3)`,
		id, e2ePickupLat+kmFromPickup*e2eKm, e2ePickupLng); err != nil {
		t.Fatalf("seed location: %v", err)
	}
}

func e2eService(db *sql.DB) *DeliveryService {
	return NewDeliveryService(
		repository.NewDeliveryRepository(db),
		repository.NewRiderRepository(db),
		ws.NewHub(),
		nil, // restaurant callback: not under test
		5.0, 5, 30,
		nil, // no Redis: PostgreSQL alone must work
	)
}

func confirmedOrderEvent(orderID int) *models.OrderPlacedEvent {
	return &models.OrderPlacedEvent{
		EventType:       "ORDER_PLACED",
		OrderID:         orderID,
		RestaurantID:    27,
		RestaurantName:  "Test Kitchen",
		RestaurantPhone: "+910000000000",
		OrderType:       "DELIVERY",
		DeliveryMode:    "platform",
		PaymentMode:     "cash",
		Amount:          250,
		Pickup:          models.LocationDetail{Latitude: e2ePickupLat, Longitude: e2ePickupLng, Address: "pickup"},
		Drop:            models.LocationDetail{Latitude: e2ePickupLat + 2*e2eKm, Longitude: e2ePickupLng, Address: "drop"},
	}
}

func requestFor(t *testing.T, db *sql.DB, orderID int, riderID string) (int, string) {
	t.Helper()
	var id int
	var status string
	if err := db.QueryRow(`SELECT request_id, status FROM delivery_order_requests WHERE order_id = $1 AND rider_id = $2`,
		orderID, riderID).Scan(&id, &status); err != nil {
		t.Fatalf("offer for %s: %v", riderID, err)
	}
	return id, status
}

func TestConfirmedOrderReachesTheRiderAndIsAssignedExclusively(t *testing.T) {
	db := testpg.Open(t)
	lines := captureDispatch(t)
	e2eSeedRider(t, db, "rider-near", 0.5)
	e2eSeedRider(t, db, "rider-farther", 1.2)
	svc := e2eService(db)
	ctx := context.Background()

	// 1. The confirmed order's event is processed.
	if err := svc.ProcessOrderPlacedEvent(ctx, confirmedOrderEvent(13294)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	// 2. The search found both riders and an offer was persisted for each.
	var status string
	if err := db.QueryRow(`SELECT delivery_status FROM delivery_orders WHERE order_id = 13294`).Scan(&status); err != nil {
		t.Fatalf("delivery order: %v", err)
	}
	if status != models.DeliveryStatusRiderSearching {
		t.Fatalf("delivery_status = %q, want rider_searching (not no_rider_found)", status)
	}
	nearID, nearStatus := requestFor(t, db, 13294, "rider-near")
	farID, _ := requestFor(t, db, 13294, "rider-farther")
	if nearStatus != models.RequestStatusPending {
		t.Fatalf("offer status %q", nearStatus)
	}

	// 3. Each offer was handed to the socket layer (no socket connected here).
	all := strings.Join(*lines, "\n")
	for _, want := range []string{
		"event=dispatch.eligibility.evaluated",
		"eligible_count=2",
		"result=ok",
		"event=dispatch.offer.persisted",
		"event=websocket.offer.lookup",
		"reason_code=no_matching_connection",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in dispatch log", want)
		}
	}

	// 4. The rider app's pending-requests poll returns the offer.
	payloads, err := svc.GetPendingRequestPayloads(ctx, "rider-near")
	if err != nil || len(payloads) != 1 || payloads[0]["request_id"] != nearID || payloads[0]["order_id"] != 13294 {
		t.Fatalf("pending requests for rider-near: %+v, %v", payloads, err)
	}
	for _, key := range []string{"customer_name", "customer_phone"} {
		if _, ok := payloads[0][key]; ok {
			t.Errorf("offer payload must not carry %s before acceptance", key)
		}
	}

	// 5. The nearer rider accepts.
	order, err := svc.AcceptRequest(ctx, nearID, "rider-near")
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	if order.AssignedRiderID == nil || *order.AssignedRiderID != "rider-near" {
		t.Fatalf("assigned to %v", order.AssignedRiderID)
	}

	// 6. The order is assigned to that rider and no longer on offer to others.
	var assigned sql.NullString
	if err := db.QueryRow(`SELECT delivery_status, assigned_rider_id FROM delivery_orders WHERE order_id = 13294`).Scan(&status, &assigned); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if status != models.DeliveryStatusRiderAssigned || assigned.String != "rider-near" {
		t.Fatalf("delivery order %s / %v", status, assigned)
	}
	if _, farStatus := requestFor(t, db, 13294, "rider-farther"); farStatus != models.RequestStatusCancelled {
		t.Fatalf("the other rider's offer is %q, want cancelled", farStatus)
	}
	if others, _ := svc.GetPendingRequestPayloads(ctx, "rider-farther"); len(others) != 0 {
		t.Fatalf("the other rider still sees %d offers", len(others))
	}
	if _, err := svc.AcceptRequest(ctx, farID, "rider-farther"); err == nil {
		t.Fatal("a second rider must not be able to accept")
	}
	var busyOrder sql.NullInt64
	if err := db.QueryRow(`SELECT current_order_id FROM rider_availability WHERE rider_id = 'rider-near'`).Scan(&busyOrder); err != nil || busyOrder.Int64 != 13294 {
		t.Fatalf("accepting rider must be busy with the order: %v %v", busyOrder, err)
	}
	if !strings.Contains(strings.Join(*lines, "\n"), "event=dispatch.offer.accepted") {
		t.Error("missing dispatch.offer.accepted")
	}
}

// Two riders accept the same order at the same instant. Exactly one wins.
func TestSimultaneousAcceptsAssignExactlyOneRider(t *testing.T) {
	db := testpg.Open(t)
	captureDispatch(t)
	e2eSeedRider(t, db, "rider-a", 0.4)
	e2eSeedRider(t, db, "rider-b", 0.6)
	svc := e2eService(db)
	ctx := context.Background()
	if err := svc.ProcessOrderPlacedEvent(ctx, confirmedOrderEvent(13400)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}
	reqA, _ := requestFor(t, db, 13400, "rider-a")
	reqB, _ := requestFor(t, db, 13400, "rider-b")

	var wg sync.WaitGroup
	results := make([]error, 2)
	start := make(chan struct{})
	for i, accept := range []struct {
		req   int
		rider string
	}{{reqA, "rider-a"}, {reqB, "rider-b"}} {
		wg.Add(1)
		go func(i, req int, rider string) {
			defer wg.Done()
			<-start
			_, results[i] = svc.AcceptRequest(ctx, req, rider)
		}(i, accept.req, accept.rider)
	}
	started := time.Now()
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d accepts succeeded (%v), want exactly 1", wins, results)
	}
	// The loser is told plainly, not with an aborted-transaction error, and
	// without waiting for PostgreSQL's deadlock detector.
	for _, err := range results {
		if err != nil && err.Error() != "order already assigned to another rider" {
			t.Fatalf("loser got %q, want \"order already assigned to another rider\"", err)
		}
	}
	if elapsed := time.Since(started); elapsed > 900*time.Millisecond {
		t.Fatalf("simultaneous accepts took %s; a deadlock was broken by timeout", elapsed)
	}
	var accepted int
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_order_requests WHERE order_id = 13400 AND status = 'accepted'`).Scan(&accepted); err != nil || accepted != 1 {
		t.Fatalf("accepted offers = %d (%v), want 1", accepted, err)
	}
}

// With no rider nearby the order is still recorded as no_rider_found, and the
// log says why (not "query failed").
func TestNoRiderNearbyIsRecordedAndExplained(t *testing.T) {
	db := testpg.Open(t)
	lines := captureDispatch(t)
	e2eSeedRider(t, db, "rider-far-away", 40)
	svc := e2eService(db)

	if err := svc.ProcessOrderPlacedEvent(context.Background(), confirmedOrderEvent(13500)); err != nil {
		t.Fatalf("ProcessOrderPlacedEvent: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT delivery_status FROM delivery_orders WHERE order_id = 13500`).Scan(&status); err != nil || status != models.DeliveryStatusNoRiderFound {
		t.Fatalf("status %q, %v", status, err)
	}
	all := strings.Join(*lines, "\n")
	if !strings.Contains(all, "event=dispatch.no_eligible_riders") || !strings.Contains(all, "rider_outside_radius:1") {
		t.Fatalf("expected an explained no-rider outcome, got:\n%s", all)
	}
	if strings.Contains(all, "eligibility_query_failed") {
		t.Fatal("the search itself must succeed")
	}
}

package service

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

func TestOnlineDispatchRepairsLostCompletionProjection(t *testing.T) {
	db, s := onlineFixture(t)
	onlineEvent(t, s, 100)
	id, _ := requestFor(t, db, 100, lcRider)
	if _, err := s.AcceptRequest(context.Background(), id, lcRider); err != nil {
		t.Fatal(err)
	}
	// Canonical completion committed, then the client/service lost the response.
	_, _ = db.Exec(`UPDATE orders SET order_status='delivered',delivered_at=NOW() WHERE order_id=100;
        UPDATE online_delivery_dispatch_config SET enabled=false`)
	for i := 0; i < 2; i++ {
		if err := s.ReconcileFoodDispatches(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	_ = db.QueryRow(`SELECT delivery_status FROM delivery_orders WHERE order_id=100`).Scan(&status)
	if status != "delivered" {
		t.Fatalf("projection stayed %s", status)
	}
	var count int
	_ = db.QueryRow(`SELECT count(*) FROM rider_earnings WHERE order_id=100`).Scan(&count)
	if count != 1 {
		t.Fatalf("earnings %d", count)
	}
	avail, err := s.deliveryRepo.GetRiderAvailability(context.Background(), lcRider)
	if err != nil || avail.CurrentOrderID != nil {
		t.Fatalf("busy after repair: %+v %v", avail, err)
	}
}

func onlineFixture(t *testing.T) (*sql.DB, *DeliveryService) {
	t.Helper()
	db := testpg.Open(t)
	if _, err := db.Exec(testpg.LifecycleSchema); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`ALTER TABLE users ADD COLUMN is_deleted boolean DEFAULT false;
        CREATE TABLE restaurants(restaurant_id bigint PRIMARY KEY,name text,metadata jsonb,latitude double precision,
          longitude double precision,street_address text,user_id uuid);
        ALTER TABLE orders ADD COLUMN is_qrunch boolean DEFAULT false,
          ADD COLUMN metadata jsonb DEFAULT '{"order_source":"customer_web"}',
          ADD COLUMN creation_source text, ADD COLUMN dining_session_id bigint, ADD COLUMN counter_id bigint,
          ADD COLUMN created_at timestamptz DEFAULT now(), ADD COLUMN updated_at timestamptz DEFAULT now(),
          ADD COLUMN picked_up_at timestamptz, ADD COLUMN delivered_at timestamptz, ADD COLUMN assigned_at timestamptz,
          ADD COLUMN rider_assigned_at timestamptz, ADD COLUMN rider_name text, ADD COLUMN rider_phone text,
          ADD COLUMN assigned_rider_name text, ADD COLUMN assigned_rider_phone text,ADD COLUMN rider_vehicle_type text,
          ADD COLUMN rider_vehicle_number text, ADD COLUMN delivery_latitude double precision DEFAULT 28.43,
          ADD COLUMN delivery_longitude double precision DEFAULT 77.04, ADD COLUMN delivery_address text DEFAULT 'Private address',
          ADD COLUMN total_amount numeric DEFAULT 250, ADD COLUMN pay_by text DEFAULT 'card',
          ADD COLUMN payment_status text DEFAULT 'pending';
        INSERT INTO restaurants VALUES(27,'Test Kitchen','{"phone":"123"}',28.4139,77.0422,'Pickup',null);
        INSERT INTO users(id,primary_role,first_name,phone) VALUES
          ('c6b46748-0000-4000-8000-000000000001','rider','First','111'),
          ('c6b46748-0000-4000-8000-000000000002','rider','Second','222');
        INSERT INTO orders(order_id,restaurant_id,order_status) VALUES(100,27,'preparing'),(101,27,'preparing');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../restaurant-service/migrations/097_online_delivery_dispatch.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE online_delivery_dispatch_config SET enabled=true`); err != nil {
		t.Fatal(err)
	}
	e2eSeedRider(t, db, lcRider, 0.1)
	e2eSeedRider(t, db, lcOtherRider, 0.2)
	svc := e2eService(db)
	t.Cleanup(func() { svc.background.Wait() })
	return db, svc
}

func onlineEvent(t *testing.T, s *DeliveryService, id int) {
	t.Helper()
	if err := s.ProcessOrderPlacedEvent(context.Background(), confirmedOrderEvent(id)); err != nil {
		t.Fatal(err)
	}
}

func TestOnlineDispatchEligibility(t *testing.T) {
	cases := map[string]string{
		"pending": `order_status='pending'`, "confirmed": `order_status='confirmed'`,
		"qrunch": `is_qrunch=true`, "qr": `metadata='{"order_source":"qr_customer"}'`,
		"manual": `metadata='{"order_source":"owner_manual"}'`, "legacy": `metadata='{}'`,
		"takeaway": `order_type='PICKUP'`, "dinein": `order_type='DINE_IN'`,
		"counter": `counter_id=5`, "offline": `creation_source='desktop_offline'`,
		"cancelled": `order_status='cancelled'`, "completed": `order_status='completed'`,
		"rejected": `order_status='rejected'`, "delivered": `order_status='delivered'`,
	}
	for name, assignment := range cases {
		t.Run(name, func(t *testing.T) {
			db, s := onlineFixture(t)
			if _, err := db.Exec(`UPDATE orders SET ` + assignment + ` WHERE order_id=100`); err != nil {
				t.Fatal(err)
			}
			onlineEvent(t, s, 100)
			var n int
			_ = db.QueryRow(`SELECT count(*) FROM delivery_orders`).Scan(&n)
			if n != 0 {
				t.Fatalf("excluded order dispatched: %s", name)
			}
		})
	}
}

func TestOnlineDispatchAtomicAcceptanceAndLostResponse(t *testing.T) {
	db, s := onlineFixture(t)
	onlineEvent(t, s, 100)
	onlineEvent(t, s, 100)
	a, _ := requestFor(t, db, 100, lcRider)
	b, _ := requestFor(t, db, 100, lcOtherRider)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, id := range []int{a, b} {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.AcceptRequest(context.Background(), id, []string{lcRider, lcOtherRider}[i])
		}(i, id)
	}
	close(start)
	wg.Wait()
	winners := 0
	winner := 0
	for i, e := range errs {
		if e == nil {
			winners++
			winner = i
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d errors=%v", winners, errs)
	}
	rider := []string{lcRider, lcOtherRider}[winner]
	req := []int{a, b}[winner]
	if _, err := s.AcceptRequest(context.Background(), req, rider); err != nil {
		t.Fatalf("lost response retry: %v", err)
	}
	var ownerRider, status, payment string
	if err := db.QueryRow(`SELECT assigned_rider_user_id,order_status,payment_status FROM orders WHERE order_id=100`).Scan(&ownerRider, &status, &payment); err != nil {
		t.Fatal(err)
	}
	if ownerRider != rider || status != "preparing" || payment != "pending" {
		t.Fatalf("projection %s %s %s", ownerRider, status, payment)
	}
	offers, err := s.GetPendingRequestPayloads(context.Background(), []string{lcOtherRider, lcRider}[winner])
	if err != nil || len(offers) != 0 {
		t.Fatalf("stale offers: %v %v", offers, err)
	}
	var count int
	_ = db.QueryRow(`SELECT count(*) FROM delivery_order_requests`).Scan(&count)
	if count != 2 {
		t.Fatalf("duplicate offers %d", count)
	}
}

func TestOnlineDispatchOneRiderTwoOrders(t *testing.T) {
	db, s := onlineFixture(t)
	onlineEvent(t, s, 100)
	onlineEvent(t, s, 101)
	a, _ := requestFor(t, db, 100, lcRider)
	b, _ := requestFor(t, db, 101, lcRider)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, id := range []int{a, b} {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.AcceptRequest(context.Background(), id, lcRider)
		}(i, id)
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, e := range errs {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("wins=%d %v", wins, errs)
	}
	// Reconnecting/go-online must not clear the workload claim.
	if err := s.deliveryRepo.UpsertRiderAvailability(context.Background(), lcRider, true, true, nil); err != nil {
		t.Fatal(err)
	}
	available, err := s.deliveryRepo.GetRiderAvailability(context.Background(), lcRider)
	if err != nil || available.IsAvailable || available.CurrentOrderID == nil {
		t.Fatalf("lost workload %+v %v", available, err)
	}
}

func TestOnlineDispatchCancellationRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		db, s := onlineFixture(t)
		onlineEvent(t, s, 100)
		id, _ := requestFor(t, db, 100, lcRider)
		var wg sync.WaitGroup
		wg.Add(2)
		start := make(chan struct{})
		go func() { defer wg.Done(); <-start; _, _ = s.AcceptRequest(context.Background(), id, lcRider) }()
		go func() {
			defer wg.Done()
			<-start
			_, _ = db.Exec(`UPDATE orders SET order_status='cancelled' WHERE order_id=100`)
		}()
		close(start)
		wg.Wait()
		offers, err := s.GetPendingRequestPayloads(context.Background(), lcOtherRider)
		if err != nil || len(offers) != 0 {
			t.Fatalf("cancelled offer visible %v %v", offers, err)
		}
		if _, err := s.AcceptRequest(context.Background(), id, lcOtherRider); err == nil {
			t.Fatal("wrong rider accepted")
		}
		_, _ = s.ReleaseClosedDeliveries(context.Background())
	}
}

func TestOnlineDispatchReconciliationPrivacyAndFlag(t *testing.T) {
	db, s := onlineFixture(t)
	if err := s.ReconcileFoodDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	offers, err := s.GetPendingRequestPayloads(context.Background(), lcRider)
	if err != nil || len(offers) != 2 {
		t.Fatalf("offers %v %v", offers, err)
	}
	for _, o := range offers {
		for _, key := range []string{"drop_latitude", "drop_longitude", "customer_phone", "customer_name"} {
			if _, exists := o[key]; exists {
				t.Fatalf("private field %s", key)
			}
		}
		if o["drop_address"] == "Private address" {
			t.Fatal("private address")
		}
	}
	_, _ = db.Exec(`UPDATE online_delivery_dispatch_config SET enabled=false`)
	id, _ := requestFor(t, db, 100, lcRider)
	if _, err := s.AcceptRequest(context.Background(), id, lcRider); err == nil {
		t.Fatal("flag off still accepted")
	}
	offers, err = s.GetPendingRequestPayloads(context.Background(), lcRider)
	if err != nil || len(offers) != 0 {
		t.Fatalf("disabled offers %v %v", offers, err)
	}
}

func TestOnlineDispatchLocationAndOffline(t *testing.T) {
	for _, change := range []string{
		`UPDATE rider_locations SET last_updated_at=now()-interval '10 minutes'`,
		`UPDATE rider_locations SET latitude=100`,
		`UPDATE rider_locations SET latitude=29`,
		`UPDATE rider_availability SET is_online=false`,
		`UPDATE users SET status='blocked'`,
	} {
		t.Run(change, func(t *testing.T) {
			db, s := onlineFixture(t)
			_, _ = db.Exec(change)
			onlineEvent(t, s, 100)
			var status string
			if err := db.QueryRow(`SELECT delivery_status FROM delivery_orders WHERE order_id=100`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != models.DeliveryStatusNoRiderFound {
				t.Fatalf("status %s", status)
			}
		})
	}
	db, s := onlineFixture(t)
	onlineEvent(t, s, 100)
	id, _ := requestFor(t, db, 100, lcRider)
	_, _ = db.Exec(`UPDATE rider_availability SET is_online=false WHERE rider_id=$1`, lcRider)
	if _, err := s.AcceptRequest(context.Background(), id, lcRider); err == nil {
		t.Fatal("offline accepted")
	}
}

func TestOnlineDispatchUnauthorizedDetailsAndActions(t *testing.T) {
	db, s := onlineFixture(t)
	onlineEvent(t, s, 100)
	if _, err := s.GetRiderOrderDetail(context.Background(), 100, lcRider, "food"); err == nil {
		t.Fatal("unassigned private details exposed")
	}
	id, _ := requestFor(t, db, 100, lcRider)
	if _, err := s.AcceptRequest(context.Background(), id, lcOtherRider); err == nil {
		t.Fatal("other rider's offer accepted")
	}
	if _, err := s.AcceptRequest(context.Background(), id, lcRider); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRiderOrderDetail(context.Background(), 100, lcOtherRider, "food"); err == nil {
		t.Fatal("loser's private details exposed")
	}
	if err := s.UpdateDeliveryStatus(context.Background(), 100, lcOtherRider, "rider_arrived_restaurant", false, "", "food"); err == nil {
		t.Fatal("unassigned action accepted")
	}
	if err := s.WithdrawDelivery(context.Background(), 100, lcOtherRider, "bad"); err == nil {
		t.Fatal("unassigned withdrawal accepted")
	}
}

func TestOnlineDispatchWithdrawalAndCompletion(t *testing.T) {
	db, s := onlineFixture(t)
	onlineEvent(t, s, 100)
	id, _ := requestFor(t, db, 100, lcRider)
	if _, err := s.AcceptRequest(context.Background(), id, lcRider); err != nil {
		t.Fatal(err)
	}
	if err := s.WithdrawDelivery(context.Background(), 100, lcRider, "vehicle issue"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRiderOrderDetail(context.Background(), 100, lcRider, "food"); err == nil {
		t.Fatal("withdrawn rider retained access")
	}
	var did int
	_ = db.QueryRow(`SELECT delivery_order_id FROM delivery_orders WHERE order_id=100`).Scan(&did)
	if _, err := s.RedispatchOrder(context.Background(), did); err != nil {
		t.Fatal(err)
	}
	other, _ := requestFor(t, db, 100, lcOtherRider)
	if _, err := s.AcceptRequest(context.Background(), other, lcOtherRider); err != nil {
		t.Fatal(err)
	}
	fake := &fakeRestaurant{t: t, db: db}
	server := httptest.NewServer(fake)
	defer server.Close()
	s.restaurantCli = client.NewRestaurantClient(server.URL, lcToken)
	ctx := context.Background()
	if err := s.UpdateDeliveryStatus(ctx, 100, lcOtherRider, "rider_arrived_restaurant", false, "", "food"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeliveryStatus(ctx, 100, lcOtherRider, "picked_up", false, "", "food"); err == nil {
		t.Fatal("pickup before ready")
	}
	_, _ = db.Exec(`UPDATE orders SET order_status='ready' WHERE order_id=100; UPDATE online_delivery_dispatch_config SET enabled=false`)
	for _, step := range []string{"picked_up", "on_the_way", "delivered"} {
		if err := s.UpdateDeliveryStatus(ctx, 100, lcOtherRider, step, false, "", "food"); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if err := s.UpdateDeliveryStatus(ctx, 100, lcOtherRider, step, false, "", "food"); err != nil {
			t.Fatalf("duplicate %s: %v", step, err)
		}
	}
	if err := s.WithdrawDelivery(ctx, 100, lcOtherRider, "late"); err == nil {
		t.Fatal("withdraw after pickup")
	}
	var payment string
	_ = db.QueryRow(`SELECT payment_status FROM orders WHERE order_id=100`).Scan(&payment)
	if payment != "pending" {
		t.Fatal("delivery paid unpaid order")
	}
	var earnings int
	if err := db.QueryRow(`SELECT count(*) FROM rider_earnings WHERE order_id=100`).Scan(&earnings); err != nil {
		t.Fatal(err)
	}
	if earnings != 1 {
		t.Fatalf("earnings count %d", earnings)
	}
}

func TestOnlineDispatchDirectReadyExpiryAndFreshLocationRecovery(t *testing.T) {
	db, s := onlineFixture(t)
	_, _ = db.Exec(`UPDATE orders SET order_status='ready' WHERE order_id=100;
        UPDATE rider_locations SET last_updated_at=NOW()-interval '10 minutes'`)
	onlineEvent(t, s, 100)
	_, _ = db.Exec(`UPDATE rider_locations SET last_updated_at=NOW()`)
	var id int
	_ = db.QueryRow(`SELECT delivery_order_id FROM delivery_orders WHERE order_id=100`).Scan(&id)
	if n, err := s.RedispatchOrder(context.Background(), id); err != nil || n != 2 {
		t.Fatalf("fresh location recovery %d %v", n, err)
	}
	_, _ = db.Exec(`UPDATE orders SET created_at=NOW()-interval '3 hours' WHERE order_id=100`)
	if err := s.ReconcileFoodDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status, delivery string
	_ = db.QueryRow(`SELECT order_status,delivery_status FROM orders WHERE order_id=100`).Scan(&status, &delivery)
	if status != "ready" || delivery != "dispatch_expired" {
		t.Fatalf("expiry cancelled customer order: %s %s", status, delivery)
	}
}

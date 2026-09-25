package service

import (
	"context"
	"sync"
	"testing"
)

func TestOnlineDispatchDeclinePreservesOrderAndOtherOffer(t *testing.T) {
	db, svc := onlineFixture(t)
	ctx := context.Background()
	onlineEvent(t, svc, 100)
	id, _ := requestFor(t, db, 100, lcRider)
	if err := svc.RejectRequest(ctx, id, lcOtherRider); err == nil {
		t.Fatal("another rider declined the offer")
	}
	for i := 0; i < 2; i++ {
		if err := svc.RejectRequest(ctx, id, lcRider); err != nil {
			t.Fatal(err)
		}
	}
	var status, payment string
	if err := db.QueryRow(`SELECT order_status,payment_status FROM orders WHERE order_id=100`).Scan(&status, &payment); err != nil {
		t.Fatal(err)
	}
	if status != "preparing" || payment != "pending" {
		t.Fatalf("decline changed customer order: %s %s", status, payment)
	}
	offers, err := svc.GetPendingRequestPayloads(ctx, lcRider)
	if err != nil || len(offers) != 0 {
		t.Fatalf("declined offer still visible: %v %v", offers, err)
	}
	other, _ := requestFor(t, db, 100, lcOtherRider)
	if _, err := svc.AcceptRequest(ctx, other, lcOtherRider); err != nil {
		t.Fatal(err)
	}
	var assigned string
	if err := db.QueryRow(`SELECT assigned_rider_user_id FROM orders WHERE order_id=100`).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned != lcOtherRider {
		t.Fatalf("canonical restaurant/customer assignment missing: %s", assigned)
	}
}

func TestOnlineDispatchExpiredDeclineCannotReportSuccess(t *testing.T) {
	db, svc := onlineFixture(t)
	onlineEvent(t, svc, 100)
	id, _ := requestFor(t, db, 100, lcRider)
	if _, err := db.Exec(`UPDATE delivery_order_requests SET expires_at=NOW()-INTERVAL '1 second' WHERE request_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.RejectRequest(context.Background(), id, lcRider); err == nil {
		t.Fatal("expired decline reported success")
	}
}

func TestOnlineDispatchAcceptDeclineRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("race", func(t *testing.T) {
			db, svc := onlineFixture(t)
			onlineEvent(t, svc, 100)
			id, _ := requestFor(t, db, 100, lcRider)
			var acceptErr, declineErr error
			var wg sync.WaitGroup
			start := make(chan struct{})
			wg.Add(2)
			go func() { defer wg.Done(); <-start; _, acceptErr = svc.AcceptRequest(context.Background(), id, lcRider) }()
			go func() { defer wg.Done(); <-start; declineErr = svc.RejectRequest(context.Background(), id, lcRider) }()
			close(start)
			wg.Wait()
			if (acceptErr == nil) == (declineErr == nil) {
				t.Fatalf("exactly one action must win: accept=%v decline=%v", acceptErr, declineErr)
			}
		})
	}
}

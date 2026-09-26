package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/push"
)

// recordingPusher stands in for push.Notifier so the dispatch wiring can be
// checked against real PostgreSQL without a phone or Google.
type recordingPusher struct {
	mu     sync.Mutex
	offers []pushedOffer
	closed []pushedClosure
}

type pushedOffer struct {
	rider string
	offer push.Offer
}

type pushedClosure struct {
	rider   string
	closure push.OfferClosure
}

func (p *recordingPusher) OfferCreated(riderID string, offer push.Offer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offers = append(p.offers, pushedOffer{riderID, offer})
}

func (p *recordingPusher) OfferClosed(riderID string, closure push.OfferClosure) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = append(p.closed, pushedClosure{riderID, closure})
}

func (p *recordingPusher) offerFor(rider string) []push.Offer {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []push.Offer
	for _, o := range p.offers {
		if o.rider == rider {
			out = append(out, o.offer)
		}
	}
	return out
}

func (p *recordingPusher) closuresFor(rider string) []push.OfferClosure {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []push.OfferClosure
	for _, c := range p.closed {
		if c.rider == rider {
			out = append(out, c.closure)
		}
	}
	return out
}

func TestPreparingAnOrderPushesTheOfferToEachEligibleRiderOnce(t *testing.T) {
	db, s := onlineFixture(t)
	pusher := &recordingPusher{}
	s.SetOfferPusher(pusher)

	onlineEvent(t, s, 100)

	for _, rider := range []string{lcRider, lcOtherRider} {
		offers := pusher.offerFor(rider)
		if len(offers) != 1 {
			t.Fatalf("rider %s got %d pushes, want 1", rider, len(offers))
		}
		offer := offers[0]

		// The push is about the persisted offer, not a parallel copy of it.
		requestID, status := requestFor(t, db, 100, rider)
		if status != "pending" || offer.RequestID != requestID {
			t.Fatalf("pushed request %d, stored %d (%s)", offer.RequestID, requestID, status)
		}
		var storedExpiry time.Time
		if err := db.QueryRow(`SELECT expires_at FROM delivery_order_requests WHERE request_id=$1`, requestID).Scan(&storedExpiry); err != nil {
			t.Fatal(err)
		}
		if !offer.ExpiresAt.Equal(storedExpiry) && offer.ExpiresAt.Sub(storedExpiry).Abs() > time.Millisecond {
			t.Fatalf("pushed expiry %v != stored %v", offer.ExpiresAt, storedExpiry)
		}

		if offer.OrderID != 100 || offer.OrderType != "food" || offer.RestaurantName != "Test Kitchen" {
			t.Fatalf("offer identity %+v", offer)
		}
		if offer.PickupAddress == "" || offer.DistanceKm <= 0 || offer.Amount != 250 || offer.PaymentMode == "" {
			t.Fatalf("offer content %+v", offer)
		}
		if offer.DeliveryDistanceKm <= 0 || !strings.HasPrefix(offer.DeliveryArea, "Approx. ") {
			t.Fatalf("delivery area %q (%v km)", offer.DeliveryArea, offer.DeliveryDistanceKm)
		}
		// The fixture's drop address is "Private address": it must not leak.
		for _, field := range []string{offer.PickupAddress, offer.DeliveryArea, offer.RestaurantName} {
			if strings.Contains(field, "Private") {
				t.Fatalf("customer address leaked into %q", field)
			}
		}
	}

	// The same ORDER_PLACED delivered twice (SQS is at-least-once) must not
	// ring anyone a second time: the pending offer is not reopened.
	onlineEvent(t, s, 100)
	for _, rider := range []string{lcRider, lcOtherRider} {
		if got := len(pusher.offerFor(rider)); got != 1 {
			t.Fatalf("duplicate event pushed again to %s: %d", rider, got)
		}
	}
}

func TestAnIneligibleRiderIsNotPushed(t *testing.T) {
	db, s := onlineFixture(t)
	pusher := &recordingPusher{}
	s.SetOfferPusher(pusher)
	// The second rider goes offline: dispatch must not offer to them, and so
	// must not wake their phone.
	if _, err := db.Exec(`UPDATE rider_availability SET is_online=false, is_available=false WHERE rider_id=$1`, lcOtherRider); err != nil {
		t.Fatal(err)
	}

	onlineEvent(t, s, 100)

	if len(pusher.offerFor(lcRider)) != 1 {
		t.Fatalf("eligible rider pushes: %d", len(pusher.offerFor(lcRider)))
	}
	if len(pusher.offerFor(lcOtherRider)) != 0 {
		t.Fatal("an offline rider's phone was woken for an offer they cannot receive")
	}
}

func TestAnOrderThatCannotBeDispatchedPushesNothing(t *testing.T) {
	db, s := onlineFixture(t)
	pusher := &recordingPusher{}
	s.SetOfferPusher(pusher)
	// Still awaiting the restaurant's acceptance: no offer may exist yet.
	if _, err := db.Exec(`UPDATE orders SET order_status='pending' WHERE order_id=100`); err != nil {
		t.Fatal(err)
	}

	onlineEvent(t, s, 100)

	if len(pusher.offers) != 0 {
		t.Fatalf("pushed %d offers for an order that is not being prepared", len(pusher.offers))
	}
}

func TestDecliningOnlyClosesThatRidersOfferAndPushesNoClosure(t *testing.T) {
	db, s := onlineFixture(t)
	pusher := &recordingPusher{}
	s.SetOfferPusher(pusher)
	onlineEvent(t, s, 100)
	a, _ := requestFor(t, db, 100, lcRider)

	if err := s.RejectRequest(context.Background(), a, lcRider); err != nil {
		t.Fatal(err)
	}

	if _, status := requestFor(t, db, 100, lcRider); status != "rejected" {
		t.Fatalf("decliner's offer is %s", status)
	}
	if _, status := requestFor(t, db, 100, lcOtherRider); status != "pending" {
		t.Fatalf("the other rider's offer must survive a decline, got %s", status)
	}
	var orderStatus string
	_ = db.QueryRow(`SELECT order_status FROM orders WHERE order_id=100`).Scan(&orderStatus)
	if orderStatus != "preparing" {
		t.Fatalf("a rider declining must not touch the customer's order: %s", orderStatus)
	}
	if len(pusher.closed) != 0 {
		t.Fatalf("closure pushes after a decline: %+v", pusher.closed)
	}
}

func TestTheRiderWhoLosesTheAcceptRaceIsToldTheOfferIsGone(t *testing.T) {
	db, s := onlineFixture(t)
	pusher := &recordingPusher{}
	s.SetOfferPusher(pusher)
	onlineEvent(t, s, 100)

	a, _ := requestFor(t, db, 100, lcRider)
	b, _ := requestFor(t, db, 100, lcOtherRider)
	riders := []string{lcRider, lcOtherRider}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i, id := range []int{a, b} {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			<-start
			_, errs[i] = s.AcceptRequest(context.Background(), id, riders[i])
		}(i, id)
	}
	close(start)
	wg.Wait()

	winner, loser := -1, -1
	for i, err := range errs {
		if err == nil {
			winner = i
		} else {
			loser = i
		}
	}
	if winner < 0 || loser < 0 {
		t.Fatalf("exactly one rider must win: %v", errs)
	}

	// The loser's own attempt is refused with a specific reason...
	if msg := errs[loser].Error(); !strings.Contains(msg, "already assigned") && !strings.Contains(msg, "already responded") {
		t.Fatalf("loser was told %q", msg)
	}
	// ...and a loser who had not tapped at all would still hear it: the closure
	// names the order so the phone can drop the offer (and its Accept button).
	if len(pusher.closuresFor(riders[winner])) != 0 {
		t.Fatal("the winner must not be told their own offer closed")
	}
	got := pusher.closuresFor(riders[loser])
	if len(got) != 1 {
		t.Fatalf("loser closure pushes = %d, want exactly 1 (%+v)", len(got), got)
	}
	if got[0].OrderID != 100 || got[0].OrderType != "food" || got[0].Reason != push.ReasonAssignedToOther {
		t.Fatalf("closure %+v", got[0])
	}
}

func TestAnOfferWithNoPusherStillWorks(t *testing.T) {
	db, s := onlineFixture(t)
	// No SetOfferPusher: push is not configured. Dispatch is unchanged.
	onlineEvent(t, s, 100)
	if _, status := requestFor(t, db, 100, lcRider); status != "pending" {
		t.Fatalf("offer %s", status)
	}
	a, _ := requestFor(t, db, 100, lcRider)
	if _, err := s.AcceptRequest(context.Background(), a, lcRider); err != nil {
		t.Fatal(err)
	}
}

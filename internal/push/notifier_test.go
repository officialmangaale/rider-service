package push

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingSender struct {
	mu      sync.Mutex
	sent    []Message
	errFor  map[string]error
	block   chan struct{}
	started chan struct{}
}

func (s *recordingSender) Send(ctx context.Context, msg Message) error {
	if s.started != nil {
		s.started <- struct{}{}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, msg)
	return s.errFor[msg.Token]
}

func (s *recordingSender) messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Message(nil), s.sent...)
	sort.Slice(out, func(i, j int) bool { return out[i].Token < out[j].Token })
	return out
}

type memoryTokens struct {
	mu      sync.Mutex
	tokens  map[string][]DeviceToken
	removed []string
	lookErr error
}

func (m *memoryTokens) DeviceTokens(_ context.Context, riderID string) ([]DeviceToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DeviceToken(nil), m.tokens[riderID]...), m.lookErr
}

func (m *memoryTokens) RemoveDeviceToken(_ context.Context, riderID, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, riderID+":"+token)
	return nil
}

func sampleOffer(expiresIn time.Duration) Offer {
	return Offer{
		RequestID: 41, OrderID: 14659, OrderType: "food",
		RestaurantName: "Spice Hub", PickupAddress: "12 MG Road, Indiranagar",
		DeliveryArea: DeliveryAreaLabel(4), DistanceKm: 1.234, DeliveryDistanceKm: 4,
		Amount: 250, PaymentMode: "cod", ExpiresAt: time.Now().Add(expiresIn),
	}
}

func TestAnOfferReachesEveryDeviceWithWhatARiderNeedsToDecide(t *testing.T) {
	sender := &recordingSender{}
	store := &memoryTokens{tokens: map[string][]DeviceToken{
		"rider-1": {{Token: "a-android", Platform: "android"}, {Token: "b-ios", Platform: "ios"}},
	}}
	n := NewNotifier(sender, store)
	n.OfferCreated("rider-1", sampleOffer(30*time.Second))
	n.Wait()

	msgs := sender.messages()
	if len(msgs) != 2 {
		t.Fatalf("messages=%d", len(msgs))
	}
	android, ios := msgs[0], msgs[1]

	d := android.Data
	want := map[string]string{
		"type": "DELIVERY_ORDER_REQUEST", "request_id": "41", "order_id": "14659", "order_ref": "#14659",
		"order_type": "food", "restaurant_name": "Spice Hub", "pickup_address": "12 MG Road, Indiranagar",
		"delivery_area": "Approx. 4 km from pickup", "distance_km": "1.23", "delivery_distance_km": "4",
		"amount": "250.00", "payment_mode": "cod",
	}
	for k, v := range want {
		if d[k] != v {
			t.Errorf("data[%s]=%q want %q", k, d[k], v)
		}
	}
	if _, err := time.Parse(time.RFC3339, d["expires_at"]); err != nil {
		t.Errorf("expires_at %q: %v", d["expires_at"], err)
	}
	if android.Alert != nil {
		t.Error("Android must be data-only: the app draws the actionable notification")
	}
	if android.CollapseKey != "offer-41" {
		t.Errorf("collapse key %q", android.CollapseKey)
	}
	// The push must never outlive the offer.
	if android.TTL <= 0 || android.TTL > 30*time.Second {
		t.Errorf("ttl %v", android.TTL)
	}

	if ios.Alert == nil || ios.APNSCategory != APNSCategoryOffer {
		t.Fatalf("iOS needs a visible alert with the action category: %+v", ios)
	}
	if !strings.Contains(ios.Alert.Body, "Spice Hub") || !strings.Contains(ios.Alert.Body, "#14659") {
		t.Errorf("alert body %q", ios.Alert.Body)
	}
}

func TestNothingThatIdentifiesTheCustomerIsPushed(t *testing.T) {
	// Offer has no field for a customer name, phone or drop address, so the
	// guarantee is structural; this pins the wire format against a regression
	// that adds one.
	sender := &recordingSender{}
	store := &memoryTokens{tokens: map[string][]DeviceToken{"r": {{Token: "t", Platform: "android"}, {Token: "u", Platform: "ios"}}}}
	n := NewNotifier(sender, store)
	n.OfferCreated("r", sampleOffer(30*time.Second))
	n.Wait()
	for _, msg := range sender.messages() {
		for key := range msg.Data {
			lower := strings.ToLower(key)
			for _, forbidden := range []string{"customer", "phone", "drop_address", "drop_lat", "drop_lng", "delivery_address", "email"} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("pushed field %q", key)
				}
			}
		}
	}
}

func TestAnAlreadyExpiredOfferIsNotPushed(t *testing.T) {
	sender := &recordingSender{}
	store := &memoryTokens{tokens: map[string][]DeviceToken{"r": {{Token: "t", Platform: "android"}}}}
	n := NewNotifier(sender, store)
	n.OfferCreated("r", sampleOffer(-time.Second))
	n.Wait()
	if len(sender.messages()) != 0 {
		t.Fatal("an expired offer was pushed")
	}
}

func TestARiderWithoutADeviceTokenIsSkippedQuietly(t *testing.T) {
	sender := &recordingSender{}
	n := NewNotifier(sender, &memoryTokens{tokens: map[string][]DeviceToken{}})
	n.OfferCreated("nobody", sampleOffer(30*time.Second))
	n.OfferCreated("", sampleOffer(30*time.Second))
	n.Wait()
	if len(sender.messages()) != 0 {
		t.Fatal("sent without a token")
	}
}

func TestATokenLookupFailureNeverPanicsOrSends(t *testing.T) {
	sender := &recordingSender{}
	n := NewNotifier(sender, &memoryTokens{lookErr: errors.New("db down")})
	n.OfferCreated("r", sampleOffer(30*time.Second))
	n.Wait()
	if len(sender.messages()) != 0 {
		t.Fatal("sent after a failed lookup")
	}
}

func TestOnlyARetiredTokenIsForgotten(t *testing.T) {
	sender := &recordingSender{errFor: map[string]error{
		"dead":  ErrUnregistered,
		"flaky": &SendError{Status: 503, Retryable: true},
	}}
	store := &memoryTokens{tokens: map[string][]DeviceToken{"r": {
		{Token: "dead", Platform: "android"}, {Token: "flaky", Platform: "android"}, {Token: "live", Platform: "android"},
	}}}
	n := NewNotifier(sender, store)
	n.OfferCreated("r", sampleOffer(30*time.Second))
	n.Wait()

	if len(sender.messages()) != 3 {
		t.Fatalf("every device should be tried, got %d", len(sender.messages()))
	}
	if len(store.removed) != 1 || store.removed[0] != "r:dead" {
		t.Fatalf("removed %v", store.removed)
	}
}

func TestDispatchNeverWaitsForTheNetwork(t *testing.T) {
	sender := &recordingSender{block: make(chan struct{}), started: make(chan struct{}, 4)}
	store := &memoryTokens{tokens: map[string][]DeviceToken{"r": {{Token: "t", Platform: "android"}}}}
	n := NewNotifier(sender, store)

	returned := make(chan struct{})
	go func() {
		n.OfferCreated("r", sampleOffer(30*time.Second))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("OfferCreated blocked on a slow send")
	}
	<-sender.started
	close(sender.block)
	n.Wait()
}

func TestABacklogDropsPushesInsteadOfBlockingDispatch(t *testing.T) {
	sender := &recordingSender{block: make(chan struct{}), started: make(chan struct{}, 200)}
	store := &memoryTokens{tokens: map[string][]DeviceToken{"r": {{Token: "t", Platform: "android"}}}}
	n := NewNotifier(sender, store)
	n.sem = make(chan struct{}, 2)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			n.OfferCreated("r", sampleOffer(30*time.Second))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch blocked behind the push backlog")
	}
	close(sender.block)
	n.Wait()
	if got := len(sender.messages()); got > 2 {
		t.Fatalf("only the bounded number of pushes may run, got %d", got)
	}
}

func TestAClosureIsADataOnlyMessageThatNamesTheOrder(t *testing.T) {
	sender := &recordingSender{}
	store := &memoryTokens{tokens: map[string][]DeviceToken{"loser": {{Token: "t", Platform: "android"}, {Token: "u", Platform: "ios"}}}}
	n := NewNotifier(sender, store)
	n.OfferClosed("loser", OfferClosure{OrderID: 14659, OrderType: "food", Reason: ReasonAssignedToOther})
	n.Wait()

	for _, msg := range sender.messages() {
		if msg.Alert != nil {
			t.Error("a closure must be silent")
		}
		if msg.Data["type"] != TypeOfferClosed || msg.Data["order_id"] != "14659" || msg.Data["reason"] != ReasonAssignedToOther {
			t.Errorf("data %v", msg.Data)
		}
		if msg.CollapseKey != "offer-closed-14659" || msg.TTL <= 0 {
			t.Errorf("collapse=%q ttl=%v", msg.CollapseKey, msg.TTL)
		}
	}
}

func TestANilNotifierIsSafe(t *testing.T) {
	var n *Notifier
	n.OfferCreated("r", sampleOffer(time.Minute))
	n.OfferClosed("r", OfferClosure{OrderID: 1})
}

func TestDeliveryAreaIsOnlyACoarseDistance(t *testing.T) {
	if got := DeliveryAreaLabel(4); got != "Approx. 4 km from pickup" {
		t.Fatalf("%q", got)
	}
	if got := DeliveryAreaLabel(0); got != "" {
		t.Fatalf("no distance, no area: %q", got)
	}
}

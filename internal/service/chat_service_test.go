package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/push"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// ---- fakes ---------------------------------------------------------------

type fakeChatStore struct {
	mu       sync.Mutex
	ctxByID  map[int]repository.ChatContext
	messages []repository.ChatMessage
	nextID   int64
}

func newFakeChatStore() *fakeChatStore {
	return &fakeChatStore{ctxByID: map[int]repository.ChatContext{}}
}

func (f *fakeChatStore) set(c repository.ChatContext) {
	f.mu.Lock()
	f.ctxByID[c.OrderID] = c
	f.mu.Unlock()
}

func (f *fakeChatStore) ResolveContext(_ context.Context, orderID int) (repository.ChatContext, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.ctxByID[orderID]
	if !ok {
		return repository.ChatContext{}, repository.ErrChatDeliveryNotFound
	}
	return c, nil
}

func (f *fakeChatStore) Insert(_ context.Context, m repository.ChatMessage) (repository.ChatMessage, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.messages {
		if existing.OrderID == m.OrderID && existing.SenderID == m.SenderID && existing.ClientMessageID == m.ClientMessageID {
			return existing, false, nil
		}
	}
	f.nextID++
	m.MessageID = f.nextID
	m.CreatedAt = time.Now()
	f.messages = append(f.messages, m)
	return m, true, nil
}

func (f *fakeChatStore) List(_ context.Context, orderID int, riderScope string, afterID int64, limit int) ([]repository.ChatMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []repository.ChatMessage{}
	for _, m := range f.messages {
		if m.OrderID != orderID || m.MessageID <= afterID {
			continue
		}
		if riderScope != "" && m.RiderID != riderScope {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeChatStore) MarkRead(_ context.Context, orderID int, viewerRole, riderScope string, upToID int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	now := time.Now()
	for i := range f.messages {
		m := &f.messages[i]
		if m.OrderID == orderID && m.MessageID <= upToID && m.ReadAt == nil && m.SenderRole != viewerRole &&
			(riderScope == "" || m.RiderID == riderScope) {
			m.ReadAt = &now
			n++
		}
	}
	return n, nil
}

func (f *fakeChatStore) Unread(_ context.Context, orderID int, viewerRole, riderScope string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.messages {
		if m.OrderID == orderID && m.ReadAt == nil && m.SenderRole != viewerRole &&
			(riderScope == "" || m.RiderID == riderScope) {
			n++
		}
	}
	return n, nil
}

func (f *fakeChatStore) HasRiderHistory(_ context.Context, orderID int, riderID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if m.OrderID == orderID && m.RiderID == riderID {
			return true, nil
		}
	}
	return false, nil
}

type recordedEvent struct {
	target string
	msg    ws.WSMessage
}

type fakeRealtime struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (f *fakeRealtime) SendToRider(riderID string, msg ws.WSMessage) {
	f.mu.Lock()
	f.events = append(f.events, recordedEvent{"rider:" + riderID, msg})
	f.mu.Unlock()
}

func (f *fakeRealtime) SendToOrder(key string, msg ws.WSMessage) {
	f.mu.Lock()
	f.events = append(f.events, recordedEvent{"order:" + key, msg})
	f.mu.Unlock()
}

func (f *fakeRealtime) targets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, e := range f.events {
		out = append(out, e.target)
	}
	return out
}

type fakePusher struct {
	riders  []string
	notices []push.ChatNotice
}

func (f *fakePusher) ChatMessage(riderID string, n push.ChatNotice) {
	f.riders = append(f.riders, riderID)
	f.notices = append(f.notices, n)
}

const (
	testOrder    = 16303
	testCustomer = "customer-A"
	testRiderA   = "rider-A"
	testRiderB   = "rider-B"
)

func activeContext(riderID, status string) repository.ChatContext {
	return repository.ChatContext{
		OrderID: testOrder, DeliveryOrderID: 7, DeliveryStatus: status,
		CustomerIDs: []string{testCustomer, ""}, RiderID: riderID,
	}
}

type chatRig struct {
	svc      *ChatService
	store    *fakeChatStore
	realtime *fakeRealtime
	pusher   *fakePusher
}

func newChatRig(c repository.ChatContext) *chatRig {
	store := newFakeChatStore()
	store.set(c)
	rt := &fakeRealtime{}
	pu := &fakePusher{}
	return &chatRig{svc: NewChatService(store, rt, pu), store: store, realtime: rt, pusher: pu}
}

func chatCode(t *testing.T, err error) string {
	t.Helper()
	var ce *ChatError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ChatError, got %v", err)
	}
	return ce.Code
}

// ---- tests ---------------------------------------------------------------

func TestChatIsClosedUntilARiderIsAssigned(t *testing.T) {
	rig := newChatRig(activeContext("", "rider_searching"))
	ctx := context.Background()

	state, err := rig.svc.Summary(ctx, testOrder, testCustomer)
	if err != nil {
		t.Fatal(err)
	}
	if state.CanSend || state.Reason != "no_rider" || state.RiderAssigned {
		t.Fatalf("no rider => no chat: %+v", state)
	}
	_, _, err = rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0001", "hello")
	if chatCode(t, err) != ChatErrNoRider {
		t.Fatalf("got %v", err)
	}
	if len(rig.store.messages) != 0 {
		t.Fatal("nothing may be stored without a rider")
	}
}

func TestCustomerAndAssignedRiderExchangeMessages(t *testing.T) {
	for _, status := range []string{"rider_assigned", "picked_up", "on_the_way"} {
		t.Run(status, func(t *testing.T) {
			rig := newChatRig(activeContext(testRiderA, status))
			ctx := context.Background()

			sent, state, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0001", "Please come to Gate 2")
			if err != nil {
				t.Fatal(err)
			}
			if !state.CanSend || sent.SenderRole != ChatRoleCustomer || sent.ID == 0 {
				t.Fatalf("customer send: %+v %+v", sent, state)
			}
			// Delivered to the ASSIGNED rider, and pushed for the background.
			if got := rig.realtime.targets(); len(got) != 1 || got[0] != "rider:"+testRiderA {
				t.Fatalf("realtime targets %v", got)
			}
			if len(rig.pusher.riders) != 1 || rig.pusher.riders[0] != testRiderA {
				t.Fatalf("push targets %v", rig.pusher.riders)
			}
			if rig.pusher.notices[0].Preview != "Please come to Gate 2" || rig.pusher.notices[0].OrderID != testOrder {
				t.Fatalf("push notice %+v", rig.pusher.notices[0])
			}

			// The rider reads it and answers.
			seen, riderState, err := rig.svc.List(ctx, testOrder, testRiderA, 0)
			if err != nil || len(seen) != 1 || riderState.Role != ChatRoleRider || riderState.UnreadCount != 1 {
				t.Fatalf("rider list: %v %+v %v", seen, riderState, err)
			}
			reply, _, err := rig.svc.Send(ctx, testOrder, testRiderA, "client-msg-0002", "Reached outside")
			if err != nil || reply.SenderRole != ChatRoleRider {
				t.Fatalf("rider send: %+v %v", reply, err)
			}
			targets := rig.realtime.targets()
			if targets[len(targets)-1] != "order:"+ChatOrderKey(testOrder) {
				t.Fatalf("a rider message goes to the customer's chat channel, got %v", targets)
			}
			// The customer sees both, in order.
			all, _, err := rig.svc.List(ctx, testOrder, testCustomer, 0)
			if err != nil || len(all) != 2 || all[0].Body != "Please come to Gate 2" || all[1].Body != "Reached outside" {
				t.Fatalf("customer list: %+v %v", all, err)
			}
		})
	}
}

func TestResendingTheSameClientMessageIDStoresAndDeliversOnce(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	ctx := context.Background()

	first, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0001", "Gate 2")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0001", "Gate 2")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("a retry must return the original message: %d vs %d", first.ID, second.ID)
	}
	if len(rig.store.messages) != 1 {
		t.Fatalf("stored %d messages", len(rig.store.messages))
	}
	if len(rig.realtime.events) != 1 || len(rig.pusher.notices) != 1 {
		t.Fatalf("a retry must not be delivered again: %d events, %d pushes", len(rig.realtime.events), len(rig.pusher.notices))
	}
}

func TestAnotherCustomerCannotReadOrSend(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	ctx := context.Background()

	if _, _, err := rig.svc.List(ctx, testOrder, "customer-B", 0); chatCode(t, err) != ChatErrForbidden {
		t.Fatalf("list: %v", err)
	}
	if _, _, err := rig.svc.Send(ctx, testOrder, "customer-B", "client-msg-0001", "hi"); chatCode(t, err) != ChatErrForbidden {
		t.Fatalf("send: %v", err)
	}
	if _, err := rig.svc.Summary(ctx, testOrder, "customer-B"); chatCode(t, err) != ChatErrForbidden {
		t.Fatalf("summary: %v", err)
	}
	if len(rig.store.messages) != 0 {
		t.Fatal("a stranger stored a message")
	}
}

func TestUnassignedRiderIsRejected(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	ctx := context.Background()

	if _, _, err := rig.svc.List(ctx, testOrder, "rider-X", 0); chatCode(t, err) != ChatErrForbidden {
		t.Fatalf("list: %v", err)
	}
	if _, _, err := rig.svc.Send(ctx, testOrder, "rider-X", "client-msg-0001", "hi"); chatCode(t, err) != ChatErrForbidden {
		t.Fatalf("send: %v", err)
	}
}

func TestOrderThatDoesNotExistIsNotFound(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	if _, err := rig.svc.Summary(context.Background(), 99999, testCustomer); chatCode(t, err) != ChatErrNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestReassignmentRoutesToTheNewRiderAndKeepsThreadsApart(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "rider_assigned"))
	ctx := context.Background()

	if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0001", "to A"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rig.svc.Send(ctx, testOrder, testRiderA, "client-msg-0002", "from A"); err != nil {
		t.Fatal(err)
	}

	// Rider A is replaced by rider B.
	rig.store.set(activeContext(testRiderB, "rider_assigned"))

	if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0003", "to B"); err != nil {
		t.Fatal(err)
	}
	targets := rig.realtime.targets()
	if got := targets[len(targets)-1]; got != "rider:"+testRiderB {
		t.Fatalf("new message must reach the CURRENT rider, got %s", got)
	}
	for _, target := range targets[2:] {
		if target == "rider:"+testRiderA {
			t.Fatal("a message was sent to the replaced rider")
		}
	}

	// The replaced rider can no longer send, and cannot see B's thread.
	if _, _, err := rig.svc.Send(ctx, testOrder, testRiderA, "client-msg-0004", "still me"); chatCode(t, err) != ChatErrForbidden {
		t.Fatalf("replaced rider send: %v", err)
	}
	aSees, aState, err := rig.svc.List(ctx, testOrder, testRiderA, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range aSees {
		if m.Body == "to B" {
			t.Fatal("replaced rider read the new rider's thread")
		}
	}
	if aState.CanSend || !aState.ReadOnly {
		t.Fatalf("replaced rider must be read-only: %+v", aState)
	}
	// B starts with a clean thread.
	bSees, _, err := rig.svc.List(ctx, testOrder, testRiderB, 0)
	if err != nil || len(bSees) != 1 || bSees[0].Body != "to B" {
		t.Fatalf("rider B sees %+v %v", bSees, err)
	}
	// The customer keeps the whole history.
	all, _, _ := rig.svc.List(ctx, testOrder, testCustomer, 0)
	if len(all) != 3 {
		t.Fatalf("customer history %d", len(all))
	}
}

func TestDeliveredChatIsReadOnly(t *testing.T) {
	for _, status := range []string{"delivered", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			rig := newChatRig(activeContext(testRiderA, "on_the_way"))
			ctx := context.Background()
			if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0001", "before"); err != nil {
				t.Fatal(err)
			}
			rig.store.set(activeContext(testRiderA, status))

			history, state, err := rig.svc.List(ctx, testOrder, testCustomer, 0)
			if err != nil || len(history) != 1 {
				t.Fatalf("history must stay readable: %v %v", history, err)
			}
			if state.CanSend || !state.ReadOnly || state.Reason != "closed" {
				t.Fatalf("state %+v", state)
			}
			if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0002", "after"); chatCode(t, err) != ChatErrClosed {
				t.Fatalf("customer send: %v", err)
			}
			if _, _, err := rig.svc.Send(ctx, testOrder, testRiderA, "client-msg-0003", "after"); chatCode(t, err) != ChatErrClosed {
				t.Fatalf("rider send: %v", err)
			}
		})
	}
}

func TestMessageValidation(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	ctx := context.Background()
	long := make([]rune, 501)
	for i := range long {
		long[i] = 'a'
	}
	for name, tc := range map[string]struct{ id, body string }{
		"empty body":     {"client-msg-0001", "   "},
		"too long":       {"client-msg-0001", string(long)},
		"missing id":     {"", "hi"},
		"short id":       {"abc", "hi"},
		"id with spaces": {"client msg 0001", "hi"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, tc.id, tc.body); chatCode(t, err) != ChatErrInvalid {
				t.Fatalf("got %v", err)
			}
		})
	}
	if len(rig.store.messages) != 0 {
		t.Fatal("an invalid message was stored")
	}
}

func TestSendRateLimit(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	ctx := context.Background()
	clock := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	rig.svc.now = func() time.Time { return clock }

	for i := 0; i < chatMaxPerMinute; i++ {
		id := "client-msg-" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "0001"
		if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, id, "hi"); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
	}
	if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-zzzzzz", "hi"); chatCode(t, err) != ChatErrRateLimited {
		t.Fatalf("expected rate limit, got %v", err)
	}
	clock = clock.Add(61 * time.Second)
	if _, _, err := rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-zzzzzz", "hi"); err != nil {
		t.Fatalf("window should reset: %v", err)
	}
}

func TestUnreadCountsAndMarkRead(t *testing.T) {
	rig := newChatRig(activeContext(testRiderA, "on_the_way"))
	ctx := context.Background()
	_, _, _ = rig.svc.Send(ctx, testOrder, testRiderA, "client-msg-0001", "one")
	last, _, _ := rig.svc.Send(ctx, testOrder, testRiderA, "client-msg-0002", "two")
	_, _, _ = rig.svc.Send(ctx, testOrder, testCustomer, "client-msg-0003", "mine")

	state, err := rig.svc.Summary(ctx, testOrder, testCustomer)
	if err != nil || state.UnreadCount != 2 {
		t.Fatalf("customer unread %d (%v)", state.UnreadCount, err)
	}
	// Own messages are never unread for the sender.
	riderState, _ := rig.svc.Summary(ctx, testOrder, testRiderA)
	if riderState.UnreadCount != 1 {
		t.Fatalf("rider unread %d", riderState.UnreadCount)
	}
	state, err = rig.svc.MarkRead(ctx, testOrder, testCustomer, last.ID)
	if err != nil || state.UnreadCount != 0 {
		t.Fatalf("after read: %d (%v)", state.UnreadCount, err)
	}
}

func TestChatChannelIsSeparateFromTracking(t *testing.T) {
	if got := ChatOrderKey(16303); got != "chat:16303" {
		t.Fatalf("chat key %q must match ws.chatKeyPrefix and never equal an order-tracking key", got)
	}
}

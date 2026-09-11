package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

type fakeFinder struct {
	candidates []repository.RedispatchCandidate
	err        error
	gotMaxAge  time.Duration
	gotCool    time.Duration
	gotLimit   int
}

func (f *fakeFinder) FindRedispatchCandidates(_ context.Context, maxAge, cooldown time.Duration, limit int) ([]repository.RedispatchCandidate, error) {
	f.gotMaxAge, f.gotCool, f.gotLimit = maxAge, cooldown, limit
	return f.candidates, f.err
}

type fakeDispatcher struct {
	mu      sync.Mutex
	offers  map[int]int
	errs    map[int]error
	visited []int
}

func (d *fakeDispatcher) RedispatchOrder(_ context.Context, deliveryOrderID int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.visited = append(d.visited, deliveryOrderID)
	if err := d.errs[deliveryOrderID]; err != nil {
		return 0, err
	}
	return d.offers[deliveryOrderID], nil
}

func TestSweepOffersEveryCandidate(t *testing.T) {
	finder := &fakeFinder{candidates: []repository.RedispatchCandidate{
		{DeliveryOrderID: 15, OrderID: 13283},
		{DeliveryOrderID: 16, OrderID: 13286},
	}}
	dispatcher := &fakeDispatcher{offers: map[int]int{15: 0, 16: 1}}
	cfg := DefaultRedispatchConfig()

	got := NewRedispatchWorker(finder, dispatcher, cfg).Sweep(context.Background())

	if got != (SweepResult{Candidates: 2, Offered: 1}) {
		t.Fatalf("unexpected result %+v", got)
	}
	if finder.gotMaxAge != cfg.MaxAge || finder.gotCool != cfg.Cooldown || finder.gotLimit != cfg.BatchSize {
		t.Fatalf("finder got maxAge=%s cooldown=%s limit=%d", finder.gotMaxAge, finder.gotCool, finder.gotLimit)
	}
}

// One broken order must not stop the others being offered.
func TestSweepContinuesPastAFailedOrder(t *testing.T) {
	finder := &fakeFinder{candidates: []repository.RedispatchCandidate{
		{DeliveryOrderID: 15, OrderID: 13283},
		{DeliveryOrderID: 16, OrderID: 13286},
	}}
	dispatcher := &fakeDispatcher{
		offers: map[int]int{16: 2},
		errs:   map[int]error{15: errors.New("boom")},
	}

	got := NewRedispatchWorker(finder, dispatcher, DefaultRedispatchConfig()).Sweep(context.Background())

	if got != (SweepResult{Candidates: 2, Offered: 2, Failed: 1}) {
		t.Fatalf("unexpected result %+v", got)
	}
	if len(dispatcher.visited) != 2 {
		t.Fatalf("expected both orders visited, got %v", dispatcher.visited)
	}
}

func TestSweepSurvivesACandidateQueryFailure(t *testing.T) {
	finder := &fakeFinder{err: errors.New("db down")}
	dispatcher := &fakeDispatcher{}

	got := NewRedispatchWorker(finder, dispatcher, DefaultRedispatchConfig()).Sweep(context.Background())

	if got != (SweepResult{}) || len(dispatcher.visited) != 0 {
		t.Fatalf("expected an empty sweep, got %+v visited=%v", got, dispatcher.visited)
	}
}

// The age ceiling must match restaurant-service's own-rider hold ceiling (2h),
// and the cooldown must outlast an offer (30s) so a lapsed offer is not
// re-rung the instant it expires.
func TestDefaultRedispatchConfigIsConservative(t *testing.T) {
	cfg := DefaultRedispatchConfig()
	if cfg.MaxAge != 2*time.Hour {
		t.Errorf("MaxAge = %s, want 2h", cfg.MaxAge)
	}
	if cfg.Cooldown < 30*time.Second {
		t.Errorf("Cooldown = %s, must be at least the 30s offer lifetime", cfg.Cooldown)
	}
	if cfg.Interval <= 0 || cfg.BatchSize <= 0 {
		t.Errorf("Interval and BatchSize must be positive: %+v", cfg)
	}
}

func TestStopIsSafeBeforeStartAndTwice(t *testing.T) {
	w := NewRedispatchWorker(&fakeFinder{}, &fakeDispatcher{}, DefaultRedispatchConfig())
	w.Stop()
	w.Stop()

	w2 := NewRedispatchWorker(&fakeFinder{}, &fakeDispatcher{}, DefaultRedispatchConfig())
	w2.Start()
	w2.Start()
	w2.Stop()
	w2.Stop()
}

package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeReleaser struct {
	calls    atomic.Int32
	released int
	err      error
}

func (f *fakeReleaser) ReleaseClosedDeliveries(context.Context) (int, error) {
	f.calls.Add(1)
	return f.released, f.err
}

func TestClosedDeliverySweepReportsReleases(t *testing.T) {
	w := NewClosedDeliveryWorker(&fakeReleaser{released: 2}, time.Hour)
	if got := w.Sweep(context.Background()); got != 2 {
		t.Fatalf("Sweep = %d, want 2", got)
	}
	failing := NewClosedDeliveryWorker(&fakeReleaser{err: errors.New("db down")}, time.Hour)
	if got := failing.Sweep(context.Background()); got != 0 {
		t.Fatalf("a failed sweep released %d", got)
	}
}

// The first sweep runs at start, not one interval later: a rider stuck since
// before a deploy is freed immediately.
func TestClosedDeliveryWorkerSweepsOnStartAndStops(t *testing.T) {
	r := &fakeReleaser{}
	w := NewClosedDeliveryWorker(r, time.Hour)
	w.Start()
	deadline := time.Now().Add(2 * time.Second)
	for r.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	w.Stop()
	w.Stop()
	if r.calls.Load() == 0 {
		t.Fatal("no sweep on start")
	}
}

package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
)

// RedispatchCandidateFinder selects orders due for another offer.
type RedispatchCandidateFinder interface {
	FindRedispatchCandidates(ctx context.Context, maxAge, cooldown time.Duration, limit int) ([]repository.RedispatchCandidate, error)
}

// OrderRedispatcher offers one order to the riders eligible now and reports
// how many offers it made.
type OrderRedispatcher interface {
	RedispatchOrder(ctx context.Context, deliveryOrderID int) (int, error)
}

// RedispatchConfig tunes the sweep. The zero value is not usable; use
// DefaultRedispatchConfig and override fields.
type RedispatchConfig struct {
	// Interval between sweeps. It bounds how long a rider who has just become
	// eligible waits for an order that is already waiting.
	Interval time.Duration
	// MaxAge is how long after the first dispatch an order is still offered.
	// Orders older than this are left alone; production holds months-old
	// orders that were never closed.
	MaxAge time.Duration
	// Cooldown is the gap after an offer expires before the order is offered
	// again, so a rider who let it lapse is not rung straight back.
	Cooldown time.Duration
	// BatchSize caps orders handled per sweep.
	BatchSize int
}

// DefaultRedispatchConfig matches the restaurant-service own-rider hold
// ceiling (2h), so an order that restaurant-service would still broadcast is
// also one this worker would still re-offer.
func DefaultRedispatchConfig() RedispatchConfig {
	return RedispatchConfig{
		Interval:  20 * time.Second,
		MaxAge:    2 * time.Hour,
		Cooldown:  60 * time.Second,
		BatchSize: 50,
	}
}

// RedispatchWorker periodically re-offers platform delivery orders that found
// no rider when they were first dispatched.
type RedispatchWorker struct {
	finder     RedispatchCandidateFinder
	dispatcher OrderRedispatcher
	cfg        RedispatchConfig

	startOnce sync.Once
	stopOnce  sync.Once
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewRedispatchWorker builds the worker. It does nothing until Start.
func NewRedispatchWorker(finder RedispatchCandidateFinder, dispatcher OrderRedispatcher, cfg RedispatchConfig) *RedispatchWorker {
	return &RedispatchWorker{
		finder:     finder,
		dispatcher: dispatcher,
		cfg:        cfg,
		stopChan:   make(chan struct{}),
	}
}

// Start begins sweeping. Calling it more than once has no further effect.
func (w *RedispatchWorker) Start() {
	w.startOnce.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			log.Printf("[REDISPATCH-WORKER] Started interval=%s max_age=%s cooldown=%s",
				w.cfg.Interval, w.cfg.MaxAge, w.cfg.Cooldown)

			ticker := time.NewTicker(w.cfg.Interval)
			defer ticker.Stop()

			for {
				select {
				case <-w.stopChan:
					log.Println("[REDISPATCH-WORKER] Stopped")
					return
				case <-ticker.C:
					w.Sweep(context.Background())
				}
			}
		}()
	})
}

// Stop ends sweeping and waits for an in-flight sweep. Safe to call more than
// once, and before Start.
func (w *RedispatchWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stopChan) })
	w.wg.Wait()
}

// SweepResult summarises one sweep, for logs and tests.
type SweepResult struct {
	Candidates int
	Offered    int
	Failed     int
}

// Sweep runs one pass. It is exported so tests can drive it without timers.
func (w *RedispatchWorker) Sweep(parent context.Context) SweepResult {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	var result SweepResult
	candidates, err := w.finder.FindRedispatchCandidates(ctx, w.cfg.MaxAge, w.cfg.Cooldown, w.cfg.BatchSize)
	if err != nil {
		log.Printf("[REDISPATCH-WORKER] Candidate query failed: %v", err)
		return result
	}
	result.Candidates = len(candidates)

	for _, c := range candidates {
		if ctx.Err() != nil {
			log.Printf("[REDISPATCH-WORKER] Sweep deadline reached after %d offers; the rest wait for the next sweep", result.Offered)
			break
		}
		offered, err := w.dispatcher.RedispatchOrder(ctx, c.DeliveryOrderID)
		if err != nil {
			result.Failed++
			log.Printf("[REDISPATCH-WORKER] Redispatch failed order_id=%d delivery_order_id=%d: %v",
				c.OrderID, c.DeliveryOrderID, err)
			continue
		}
		result.Offered += offered
	}

	// Quiet when nothing happened: an order waiting for a rider is normal and
	// would otherwise log every Interval.
	if result.Offered > 0 || result.Failed > 0 {
		log.Printf("[REDISPATCH-WORKER] Sweep candidates=%d offers=%d failed=%d",
			result.Candidates, result.Offered, result.Failed)
	}
	return result
}

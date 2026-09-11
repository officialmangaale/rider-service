package worker

import (
	"context"
	"log"
	"sync"
	"time"
)

// ClosedDeliveryReleaser frees riders whose delivery the restaurant ended.
type ClosedDeliveryReleaser interface {
	ReleaseClosedDeliveries(ctx context.Context) (int, error)
}

// ClosedDeliveryWorker periodically releases active deliveries whose
// restaurant order was completed, cancelled or rejected by the owner, so the
// rider is offered orders again even if their app never asks.
type ClosedDeliveryWorker struct {
	releaser ClosedDeliveryReleaser
	interval time.Duration

	startOnce sync.Once
	stopOnce  sync.Once
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

func NewClosedDeliveryWorker(releaser ClosedDeliveryReleaser, interval time.Duration) *ClosedDeliveryWorker {
	return &ClosedDeliveryWorker{releaser: releaser, interval: interval, stopChan: make(chan struct{})}
}

// Start begins sweeping, once immediately. Calling it again has no effect.
func (w *ClosedDeliveryWorker) Start() {
	w.startOnce.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			log.Printf("[CLOSED-DELIVERY-WORKER] Started interval=%s", w.interval)
			ticker := time.NewTicker(w.interval)
			defer ticker.Stop()
			for {
				w.Sweep(context.Background())
				select {
				case <-w.stopChan:
					log.Println("[CLOSED-DELIVERY-WORKER] Stopped")
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

// Stop ends sweeping and waits for an in-flight sweep.
func (w *ClosedDeliveryWorker) Stop() {
	w.stopOnce.Do(func() { close(w.stopChan) })
	w.wg.Wait()
}

// Sweep runs one pass and returns how many deliveries it released.
func (w *ClosedDeliveryWorker) Sweep(parent context.Context) int {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	released, err := w.releaser.ReleaseClosedDeliveries(ctx)
	if err != nil {
		log.Printf("[CLOSED-DELIVERY-WORKER] sweep failed: %v", err)
	}
	return released
}

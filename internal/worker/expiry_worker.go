package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// ExpiryWorker periodically checks for expired delivery order requests.
type ExpiryWorker struct {
	repo     *repository.DeliveryRepository
	hub      *ws.Hub
	interval time.Duration
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewExpiryWorker creates a new worker.
func NewExpiryWorker(repo *repository.DeliveryRepository, hub *ws.Hub, interval time.Duration) *ExpiryWorker {
	return &ExpiryWorker{
		repo:     repo,
		hub:      hub,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins the periodic expiration check.
func (w *ExpiryWorker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		log.Printf("[EXPIRY-WORKER] Started with interval %s", w.interval)

		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopChan:
				log.Println("[EXPIRY-WORKER] Stopped")
				return
			case <-ticker.C:
				w.processExpirations()
			}
		}
	}()
}

// Stop gracefully stops the worker.
func (w *ExpiryWorker) Stop() {
	close(w.stopChan)
	w.wg.Wait()
}

func (w *ExpiryWorker) processExpirations() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expiredReqs, err := w.repo.ExpirePendingRequests(ctx)
	if err != nil {
		log.Printf("[EXPIRY-WORKER] Error expiring requests: %v", err)
		return
	}

	if len(expiredReqs) > 0 {
		log.Printf("[EXPIRY-WORKER] Found %d expired requests", len(expiredReqs))
	}

	deliveryOrdersToCheck := make(map[int]bool)

	for _, req := range expiredReqs {
		deliveryOrdersToCheck[req.DeliveryOrderID] = true

		// Notify rider that request expired
		w.hub.SendToRider(req.RiderID, ws.WSMessage{
			Type: "ORDER_REQUEST_EXPIRED",
			Data: map[string]interface{}{
				"request_id": req.RequestID,
				"order_id":   req.OrderID,
			},
		})
		log.Printf("[EXPIRY-WORKER] Notified rider %s about expired request %d", req.RiderID, req.RequestID)
	}

	// For each delivery order that had an expired request, check if it needs to transition to 'no_rider_found'
	for deliveryOrderID := range deliveryOrdersToCheck {
		w.checkAllRequestsDone(ctx, deliveryOrderID)
	}
}

func (w *ExpiryWorker) checkAllRequestsDone(ctx context.Context, deliveryOrderID int) {
	pending, err := w.repo.CountPendingForOrder(ctx, deliveryOrderID)
	if err != nil {
		return
	}
	if pending == 0 {
		hasAccepted, err := w.repo.HasAcceptedRequest(ctx, deliveryOrderID)
		if err == nil && !hasAccepted {
			// All requests expired/rejected and none accepted
			_ = w.repo.UpdateDeliveryStatus(ctx, nil, deliveryOrderID, "no_rider_found")
			log.Printf("[EXPIRY-WORKER] All requests expired/rejected for delivery_order %d, marked no_rider_found", deliveryOrderID)
			
			// If we wanted to trigger an automatic radius expansion, we could do it here
			// by reading the delivery order and running the nearest riders search again.
		}
	}
}

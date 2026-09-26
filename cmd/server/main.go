package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	dispatchcache "github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/cache"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/config"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/database"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/router"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/worker"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

func main() {
	// Load .env if present (dev mode)
	_ = godotenv.Load("../../.env")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Config load failed: %v", err)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Database connection failed: %v", err)
	}
	defer db.Close()
	log.Println("[INFO] Database connected")

	// --- Initialize new delivery components ---
	hub := ws.NewHub()

	restaurantCli := client.NewRestaurantClient(cfg.RestaurantServiceBaseURL, cfg.InternalServiceToken)

	deliveryRepo := repository.NewDeliveryRepository(db)
	riderRepo := repository.NewRiderRepository(db)

	// A database that predates migrations 078/095/096/097 makes every food
	// dispatch fail with nothing but "Searching for a rider" on the restaurant
	// screen. Say so once, loudly, at boot. Log-only: it never stops the service.
	schemaCtx, cancelSchema := context.WithTimeout(context.Background(), 5*time.Second)
	if gaps, err := deliveryRepo.DispatchSchemaGaps(schemaCtx); err != nil {
		log.Printf("[WARN] Dispatch schema check failed: %v", err)
	} else if len(gaps) > 0 {
		log.Printf("[ERROR] ONLINE FOOD DISPATCH CANNOT RUN: database is missing [%s]. Apply restaurant-service/migrations 078, 095, 096, 097 in that order, then enable dispatch for a restaurant allowlist (docs/online-delivery-flow.md). Until then no food offer can be created, listed or accepted.", strings.Join(gaps, "; "))
	} else {
		log.Println("[INFO] Dispatch schema ready (078, 095, 096, 097)")
	}
	cancelSchema()

	dispatchCache, err := dispatchcache.NewRedisDispatchCache(cfg.RedisURL)
	if err != nil {
		log.Printf("[WARN] Redis dispatch cache disabled: %v", err)
	} else if dispatchCache != nil {
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := dispatchCache.Ping(pingCtx); err != nil {
			log.Printf("[WARN] Redis dispatch cache ping failed, continuing with SQL fallback: %v", err)
			_ = dispatchCache.Close()
			dispatchCache = nil
		} else {
			log.Println("[INFO] Redis dispatch cache enabled")
			defer dispatchCache.Close()
		}
		cancel()
	}

	deliverySvc := service.NewDeliveryService(
		deliveryRepo,
		riderRepo,
		hub,
		restaurantCli,
		cfg.SearchRadiusKm,
		cfg.MaxRidersToNotify,
		cfg.RequestExpirySeconds,
		dispatchCache,
	)

	// Rider referral qualification. Registering the flag is additive: with
	// RIDER_REFERRAL_ENABLED unset, delivery completion behaves exactly as it
	// did before the referral programme existed.
	deliverySvc.SetRiderReferralEnabled(cfg.RiderReferralEnabled)
	// Opt-in per-target dispatch trace; off unless DISPATCH_TRACE_UNTIL is a
	// future time. See docs/rider-offer-investigation/OBSERVABILITY_PLAN.md.
	deliverySvc.SetTrace(dispatchtrace.LoadTraceFromEnv())
	if cfg.RiderReferralEnabled {
		log.Println("[INIT] Rider referral qualification callback enabled")
	}

	// --- Initialize workers ---
	var sqsConsumer *worker.SQSConsumer
	if cfg.SQSOrdersQueueURL != "" {
		sqsConsumer, err = worker.NewSQSConsumer(cfg.AWSRegion, cfg.SQSOrdersQueueURL, deliverySvc)
		if err != nil {
			log.Fatalf("[FATAL] Failed to initialize SQS consumer: %v", err)
		}
		sqsConsumer.Start()
	} else {
		log.Println("[WARN] SQS_ORDERS_QUEUE_URL not set. SQS consumer disabled.")
	}

	expiryWorker := worker.NewExpiryWorker(deliveryRepo, hub, dispatchCache, 10*time.Second)
	expiryWorker.Start()

	// Frees riders whose delivery the restaurant completed/cancelled/rejected,
	// so they are offered orders again.
	closedDeliveryWorker := worker.NewClosedDeliveryWorker(deliverySvc, 30*time.Second)
	closedDeliveryWorker.Start()

	var redispatchWorker *worker.RedispatchWorker
	if cfg.RedispatchIntervalSeconds > 0 {
		redispatchCfg := worker.DefaultRedispatchConfig()
		redispatchCfg.Interval = time.Duration(cfg.RedispatchIntervalSeconds) * time.Second
		redispatchWorker = worker.NewRedispatchWorker(deliveryRepo, deliverySvc, redispatchCfg)
		redispatchWorker.Start()
	} else {
		log.Println("[WARN] REDISPATCH_INTERVAL_SECONDS=0. Orders that find no rider at dispatch will not be offered again.")
	}

	engine := router.Setup(db, cfg, hub, deliverySvc, restaurantCli)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("[INFO] Rider service starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[INFO] Shutting down...")

	// Stop workers
	if sqsConsumer != nil {
		sqsConsumer.Stop()
	}
	expiryWorker.Stop()
	closedDeliveryWorker.Stop()
	if redispatchWorker != nil {
		redispatchWorker.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Shutdown failed: %v", err)
	}
	log.Println("[INFO] Rider service stopped")
}

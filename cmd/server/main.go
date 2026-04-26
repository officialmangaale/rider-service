package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/config"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/database"
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
	
	deliverySvc := service.NewDeliveryService(
		deliveryRepo,
		riderRepo,
		hub,
		restaurantCli,
		cfg.SearchRadiusKm,
		cfg.MaxRidersToNotify,
		cfg.RequestExpirySeconds,
	)

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

	expiryWorker := worker.NewExpiryWorker(deliveryRepo, hub, 10*time.Second)
	expiryWorker.Start()

	engine := router.Setup(db, cfg, hub, deliverySvc)

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[FATAL] Shutdown failed: %v", err)
	}
	log.Println("[INFO] Rider service stopped")
}

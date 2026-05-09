package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all service configuration from environment variables.
type Config struct {
	Port           string
	DatabaseURL    string
	JWTSecret      string // Shared secret with user-service for JWT verification
	JWTExpiryHours int    // Not used for issuance, only for reference

	// SQS Consumer
	SQSOrdersQueueURL string
	AWSRegion          string

	// Internal service communication
	RestaurantServiceBaseURL string
	InternalServiceToken     string

	// Delivery config
	SearchRadiusKm    float64
	MaxRidersToNotify int
	RequestExpirySeconds int
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:           getEnv("PORT", "8084"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 24),

		// SQS
		SQSOrdersQueueURL: os.Getenv("SQS_ORDERS_QUEUE_URL"),
		AWSRegion:          getEnv("AWS_REGION", "ap-south-1"),

		// Internal
		RestaurantServiceBaseURL: os.Getenv("RESTAURANT_SERVICE_INTERNAL_BASE_URL"),
		InternalServiceToken:     os.Getenv("INTERNAL_SERVICE_TOKEN"),

		// Delivery defaults
		SearchRadiusKm:       getEnvFloat("SEARCH_RADIUS_KM", 5.0),
		MaxRidersToNotify:    getEnvInt("MAX_RIDERS_TO_NOTIFY", 5),
		RequestExpirySeconds: getEnvInt("REQUEST_EXPIRY_SECONDS", 30),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required (must match user-service)")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

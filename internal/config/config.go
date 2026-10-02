package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all service configuration from environment variables.
type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string // Shared secret with user-service for JWT verification
	JWTExpiryHours int    // Not used for issuance, only for reference

	// SQS Consumer
	SQSOrdersQueueURL string
	AWSRegion         string

	// Internal service communication
	RestaurantServiceBaseURL string
	InternalServiceToken     string

	// RiderReferralEnabled gates the rider-referral qualification callback to
	// restaurant-service. Defaults to false: with it off, delivery completion
	// behaves exactly as it did before the referral programme existed.
	RiderReferralEnabled bool

	// Delivery config
	SearchRadiusKm       float64
	MaxRidersToNotify    int
	RequestExpirySeconds int

	// RedispatchIntervalSeconds is how often unmatched platform orders are
	// offered again. 0 turns re-dispatch off, restoring the old behaviour
	// where an order that found no rider at dispatch stayed unmatched.
	RedispatchIntervalSeconds int

	// Device push (FCM HTTP v1) for delivery offers. Off unless a service
	// account is configured: without one, offers still reach riders over the
	// socket and the app's polling, exactly as before.
	//
	// The service account must belong to the Firebase project of the rider
	// app (its google-services.json project_id) and be allowed to send
	// messages (role "Firebase Cloud Messaging API Admin" or "Firebase Admin").
	FCMServiceAccount string // FCM_SERVICE_ACCOUNT_JSON: JSON, or base64 of it
	FCMProjectID      string // FCM_PROJECT_ID: optional, defaults to the account's project_id
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:           getEnv("PORT", "8084"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RedisURL:       os.Getenv("REDIS_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 24),

		// SQS
		SQSOrdersQueueURL: firstNonEmptyEnv("SQS_ORDERS_QUEUE_URL", "SQS_RIDER_QUEUE_URL"),
		AWSRegion:         getEnv("AWS_REGION", "ap-south-1"),

		// Internal
		RestaurantServiceBaseURL: os.Getenv("RESTAURANT_SERVICE_INTERNAL_BASE_URL"),
		InternalServiceToken:     os.Getenv("INTERNAL_SERVICE_TOKEN"),

		// Referral programme — off by default.
		RiderReferralEnabled: getEnvBool("RIDER_REFERRAL_ENABLED", false),

		// Delivery defaults
		SearchRadiusKm:       getEnvFloat("SEARCH_RADIUS_KM", 5.0),
		MaxRidersToNotify:    getEnvInt("MAX_RIDERS_TO_NOTIFY", 5),
		RequestExpirySeconds: getEnvInt("REQUEST_EXPIRY_SECONDS", 30),

		RedispatchIntervalSeconds: getEnvInt("REDISPATCH_INTERVAL_SECONDS", 20),

		FCMServiceAccount: firstNonEmptyEnv("FCM_SERVICE_ACCOUNT_JSON"),
		FCMProjectID:      firstNonEmptyEnv("FCM_PROJECT_ID"),
	}
	// A key file is the usual way to hand a service account to a container.
	if cfg.FCMServiceAccount == "" {
		if path := firstNonEmptyEnv("FCM_SERVICE_ACCOUNT_FILE", "GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("FCM service account file %q: %w", path, err)
			}
			cfg.FCMServiceAccount = string(raw)
		}
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required (must match user-service)")
	}

	return cfg, nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
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

func getEnvBool(key string, fallback bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

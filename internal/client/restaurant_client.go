package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// RestaurantClient handles internal HTTP callbacks to restaurant-service.
type RestaurantClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// AssignRiderPayload is the body sent to restaurant-service when a rider is assigned.
type AssignRiderPayload struct {
	RiderID       string `json:"rider_id"`
	RiderName     string `json:"rider_name"`
	RiderPhone    string `json:"rider_phone"`
	VehicleType   string `json:"vehicle_type"`
	VehicleNumber string `json:"vehicle_number"`
	AssignedAt    string `json:"assigned_at"`
}

// DeliveryStatusPayload is the body sent to restaurant-service when delivery status changes.
type DeliveryStatusPayload struct {
	OrderID          int    `json:"order_id"`
	RestaurantID     int    `json:"restaurant_id"`
	RiderID          string `json:"rider_user_id"`
	DeliveryStatus   string `json:"delivery_status"`
	PaymentCollected bool   `json:"payment_collected"`
	Notes            string `json:"notes,omitempty"`
}

// NewRestaurantClient creates a new RestaurantClient.
func NewRestaurantClient(baseURL, token string) *RestaurantClient {
	return &RestaurantClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifyRiderAssigned calls POST {baseURL}/internal/orders/{orderId}/assign-rider
// to inform restaurant-service that a rider has been assigned.
func (c *RestaurantClient) NotifyRiderAssigned(orderID int, payload AssignRiderPayload) error {
	if c.baseURL == "" {
		log.Printf("[RESTAURANT-CLIENT] Base URL not configured, skipping callback for order %d", orderID)
		return nil
	}

	url := fmt.Sprintf("%s/internal/orders/%d/assign-rider", c.baseURL, orderID)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal assign-rider payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("callback to restaurant-service failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[RESTAURANT-CLIENT] Rider assignment callback success for order %d (status %d)", orderID, resp.StatusCode)
		return nil
	}

	return fmt.Errorf("restaurant-service returned status %d for order %d", resp.StatusCode, orderID)
}

// NotifyRiderAssignedAsync calls the callback asynchronously with retry.
func (c *RestaurantClient) NotifyRiderAssignedAsync(orderID int, payload AssignRiderPayload) {
	go func() {
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			err := c.NotifyRiderAssigned(orderID, payload)
			if err == nil {
				return
			}
			log.Printf("[RESTAURANT-CLIENT] Attempt %d/%d failed for order %d: %v", attempt, maxRetries, orderID, err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second) // exponential-ish backoff
			}
		}
		log.Printf("[RESTAURANT-CLIENT] All retries exhausted for order %d, assignment kept locally", orderID)
	}()
}

// NotifyDeliveryStatusUpdate calls POST {baseURL}/internal/orders/{orderId}/delivery-status
func (c *RestaurantClient) NotifyDeliveryStatusUpdate(orderID int, payload DeliveryStatusPayload) error {
	if c.baseURL == "" {
		log.Printf("[RESTAURANT-CLIENT] Base URL not configured, skipping status callback for order %d", orderID)
		return nil
	}

	url := fmt.Sprintf("%s/internal/orders/%d/delivery-status", c.baseURL, orderID)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal status payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("status callback to restaurant-service failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("restaurant-service returned status %d for order %d", resp.StatusCode, orderID)
}

// NotifyDeliveryStatusUpdateAsync calls the callback asynchronously with retry.
func (c *RestaurantClient) NotifyDeliveryStatusUpdateAsync(orderID int, payload DeliveryStatusPayload) {
	go func() {
		maxRetries := 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			err := c.NotifyDeliveryStatusUpdate(orderID, payload)
			if err == nil {
				return
			}
			log.Printf("[RESTAURANT-CLIENT] Status callback attempt %d/%d failed for order %d: %v", attempt, maxRetries, orderID, err)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}
		}
	}()
}

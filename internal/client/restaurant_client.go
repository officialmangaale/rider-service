package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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
//
// Trailing slashes are trimmed from baseURL. Production configured
// "https://restaurant-prod.mangaale.com/", so every callback went to
// "//internal/orders/…", which restaurant-service's router answers with 404:
// the rider was never recorded on the customer's order and every pickup failed.
func NewRestaurantClient(baseURL, token string) *RestaurantClient {
	return &RestaurantClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CallbackError is a non-2xx answer from restaurant-service. Message is that
// service's own `message` field (fixed phrases and status names, no personal
// data), so callers can log and surface why a callback was refused.
type CallbackError struct {
	StatusCode int
	OrderID    int
	Message    string
}

func (e *CallbackError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("restaurant-service returned status %d for order %d", e.StatusCode, e.OrderID)
	}
	return fmt.Sprintf("restaurant-service returned status %d for order %d: %s", e.StatusCode, e.OrderID, e.Message)
}

// maxCallbackMessage bounds what is kept from a refusal body.
const maxCallbackMessage = 160

func callbackError(resp *http.Response, orderID int) *CallbackError {
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body)
	msg := strings.Join(strings.Fields(body.Message), " ")
	if len(msg) > maxCallbackMessage {
		msg = msg[:maxCallbackMessage]
	}
	return &CallbackError{StatusCode: resp.StatusCode, OrderID: orderID, Message: msg}
}

// NotifyRiderAssigned calls POST {baseURL}/internal/orders/{orderId}/assign-rider
// to inform restaurant-service that a rider has been assigned.
func (c *RestaurantClient) NotifyRiderAssigned(orderID int, payload AssignRiderPayload) error {
	if c.baseURL == "" {
		return fmt.Errorf("restaurant-service base URL is not configured")
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

	return callbackError(resp, orderID)
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
		return fmt.Errorf("restaurant-service base URL is not configured")
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

	return callbackError(resp, orderID)
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

// RiderDeliveryCompletedPayload is the body sent to restaurant-service when a
// rider completes a delivery, so the referral domain can decide whether that
// delivery qualifies a rider referral.
//
// It carries a fact, not a decision: this service owns delivery completion and
// the rider's running delivery count, while the rules that turn those into a
// reward live in restaurant-service.
type RiderDeliveryCompletedPayload struct {
	// RiderID is the rider's auth user id — the same subject the referral
	// domain keys on.
	RiderID string `json:"rider_id"`
	// DeliveryRef identifies this delivery, and is what makes the call
	// idempotent: the outbox dedupes on it, so a retry cannot qualify a
	// referral twice.
	DeliveryRef         string `json:"delivery_ref"`
	CompletedDeliveries int    `json:"completed_deliveries"`
}

// NotifyRiderDeliveryCompleted calls
// POST {baseURL}/internal/referrals/rider-delivery-completed.
func (c *RestaurantClient) NotifyRiderDeliveryCompleted(payload RiderDeliveryCompletedPayload) error {
	if c.baseURL == "" {
		return fmt.Errorf("restaurant-service base URL is not configured")
	}

	url := fmt.Sprintf("%s/internal/referrals/rider-delivery-completed", c.baseURL)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal referral payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("referral callback to restaurant-service failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("restaurant-service returned status %d for rider referral callback", resp.StatusCode)
}

// NotifyRiderDeliveryCompletedAsync fires the callback in the background.
//
// A referral is worth strictly less than a delivery, so this never blocks and
// never reports failure to the caller. It retries a few times and then gives
// up loudly in the log: the delivery itself has already been committed, and a
// missed referral is recoverable by hand, whereas a failed delivery is not.
//
// The rider id is deliberately not logged on failure — only the delivery
// reference, which is not personal data.
func (c *RestaurantClient) NotifyRiderDeliveryCompletedAsync(payload RiderDeliveryCompletedPayload) {
	if c.baseURL == "" {
		return
	}
	go func() {
		const maxRetries = 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := c.NotifyRiderDeliveryCompleted(payload); err == nil {
				return
			} else if attempt == maxRetries {
				log.Printf("[RESTAURANT-CLIENT] Rider referral callback gave up after %d attempts for delivery %s: %v",
					maxRetries, payload.DeliveryRef, err)
			} else {
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}
		}
	}()
}

// ────────────────────────────────────────────────────────────────────────────
// Grocery deliveries (Phase 6)
//
// The same internal-token guard and the same retry behaviour as the food
// callbacks above; only the path and the payload differ, because a grocery
// order lives in its own table with its own status machine.
// ────────────────────────────────────────────────────────────────────────────

// GroceryAssignRiderPayload is the rider recorded on a grocery order.
type GroceryAssignRiderPayload struct {
	RiderID    string `json:"rider_id"`
	RiderName  string `json:"rider_name,omitempty"`
	RiderPhone string `json:"rider_phone,omitempty"`
}

// GroceryDeliveryStatusPayload is a rider status report for a grocery order.
type GroceryDeliveryStatusPayload struct {
	RiderID        string `json:"rider_id"`
	DeliveryStatus string `json:"delivery_status"`
	Reason         string `json:"reason,omitempty"`
}

// NotifyGroceryRiderAssigned calls
// POST {baseURL}/internal/grocery/orders/{orderId}/assign-rider.
//
// A 409 means the order is no longer available — the shop assigned its own
// rider, or it moved on — and the caller must undo its own assignment.
func (c *RestaurantClient) NotifyGroceryRiderAssigned(orderID int, payload GroceryAssignRiderPayload) error {
	return c.postGrocery(orderID, fmt.Sprintf("%s/internal/grocery/orders/%d/assign-rider", c.baseURL, orderID), payload)
}

// NotifyGroceryDeliveryStatus calls
// POST {baseURL}/internal/grocery/orders/{orderId}/delivery-status.
func (c *RestaurantClient) NotifyGroceryDeliveryStatus(orderID int, payload GroceryDeliveryStatusPayload) error {
	return c.postGrocery(orderID, fmt.Sprintf("%s/internal/grocery/orders/%d/delivery-status", c.baseURL, orderID), payload)
}

func (c *RestaurantClient) postGrocery(orderID int, url string, payload interface{}) error {
	if c.baseURL == "" {
		return fmt.Errorf("restaurant-service base URL is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal grocery payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("grocery callback to restaurant-service failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return callbackError(resp, orderID)
}

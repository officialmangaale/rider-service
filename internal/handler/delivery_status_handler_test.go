package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// The rider app reads `error_code` (ApiException.errorCode) to tell "kitchen
// not ready" apart from a real failure. HTTP codes stay 409/400 so older app
// builds, which refresh on those, behave as before.
func TestStatusRefusalCarriesAnErrorCodeAndTheCurrentStatus(t *testing.T) {
	db := testpg.Open(t)
	if _, err := db.Exec(testpg.LifecycleSchema); err != nil {
		t.Fatal(err)
	}
	const rider = "c6b46748-0000-4000-8000-000000000001"
	if _, err := db.Exec(`INSERT INTO orders (order_id, restaurant_id, order_status, assigned_rider_user_id) VALUES (900, 27, 'preparing', $1)`, rider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO delivery_orders (order_id, restaurant_id, customer_id, pickup_latitude, pickup_longitude, drop_latitude, drop_longitude,
			delivery_status, assigned_rider_id, rider_user_id, restaurant_name, restaurant_phone)
		VALUES (900, 27, 5, 28.4, 77.0, 28.5, 77.1, 'rider_arrived_restaurant', $1, $1, 'Kitchen', '')`, rider); err != nil {
		t.Fatal(err)
	}
	svc := service.NewDeliveryService(repository.NewDeliveryRepository(db), repository.NewRiderRepository(db), ws.NewHub(), nil, 5, 5, 30, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/riders/orders/:orderId/status", func(c *gin.Context) {
		c.Set("user_id", rider)
		NewDeliveryHandler(svc).UpdateDeliveryStatus(c)
	})

	post := func(body string) (int, map[string]interface{}) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/riders/orders/900/status", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		var out map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	// No restaurant client: the refusal must still be a clean coded error.
	code, body := post(`{"delivery_status":"picked_up"}`)
	if code != http.StatusConflict || body["error_code"] != service.ErrCodeRestaurantSync {
		t.Fatalf("no client: %d %v", code, body)
	}

	code, body = post(`{"delivery_status":"delivered","payment_collected":true}`)
	if code != http.StatusConflict || body["error_code"] != service.ErrCodeInvalidTransition {
		t.Fatalf("skip to delivered: %d %v", code, body)
	}
	data, _ := body["data"].(map[string]interface{})
	if data["current_status"] != "rider_arrived_restaurant" || data["requested_status"] != "delivered" {
		t.Fatalf("data %v", data)
	}
}

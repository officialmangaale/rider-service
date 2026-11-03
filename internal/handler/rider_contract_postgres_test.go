package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/models"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestContractOfferFreezesSalaryIncentiveAndCancellationRequiresApproval(t *testing.T) {
	db := membershipDB(t)
	migration, err := os.ReadFile("../testpg/testdata/147_rider_earning_contracts.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", supRider1) })
	admin := NewAdminRiderHandler(db)
	admin.ContractPayEnabled = true
	router.PUT("/contract/:riderId", admin.SaveEarningContract)
	router.POST("/review/:reviewId", admin.ReviewCancellationPay)
	router.POST("/salary/:riderId", admin.AccrueSalary)
	call := func(method, path, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	call("PUT", "/contract/"+supRider1, `{"enabled":true,"mode":"salary","salary_period":"monthly","salary_cents":2000000,"incentive_permille":100,"cancellation":{"picked_up":{"incentive_permille":500}}}`, 200)
	call("PUT", "/contract/"+supRider1, `{"enabled":true,"mode":"salary","salary_period":"monthly","salary_cents":2000000,"base_cents":100}`, 400)
	if _, err = db.Exec(`CREATE TABLE order_pricing_snapshots(order_id bigint PRIMARY KEY,rider_cost_cents bigint,breakdown jsonb);
 INSERT INTO order_pricing_snapshots VALUES(91,3000,'{"customer_item_total":500,"coupon_discount":50,"referral_discount":25,"tip_amount":10,"delivery_distance_km":3}');
 INSERT INTO delivery_orders(order_id,order_type,restaurant_id,customer_id,pickup_latitude,pickup_longitude,drop_latitude,drop_longitude,delivery_status)VALUES(91,'food',1,1,28,77,28.1,77.1,'rider_searching');`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewDeliveryRepository(db)
	repo.ContractPayEnabled = true
	var deliveryID int
	if err = db.QueryRow(`SELECT delivery_order_id FROM delivery_orders WHERE order_id=91`).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	var reqID int
	// Use the repository path that freezes the offer under the membership lock.
	request, err := repo.CreateRequest(context.Background(), deliveryID, 91, supRider1, 1.0, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	reqID = request.RequestID
	quote, err := repo.RequestPay(context.Background(), request)
	if err != nil || quote == nil || *quote != 52.5 {
		t.Fatal(quote, err)
	}
	if _, err = db.Exec(`UPDATE rider_offer_pay SET amount_cents=999 WHERE request_id=$1`, reqID); err == nil {
		t.Fatal("offer pay snapshot is mutable")
	}
	call("PUT", "/contract/"+supRider1, `{"enabled":true,"mode":"salary","salary_period":"monthly","salary_cents":2000000,"incentive_cents":1000,"version":1,"cancellation":{"picked_up":{"incentive_permille":500}}}`, 200)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = repo.AcceptRequest(context.Background(), tx, reqID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE delivery_orders SET assigned_rider_id=$2,delivery_status='picked_up' WHERE delivery_order_id=$1`, deliveryID, supRider1); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	order := &models.DeliveryOrder{DeliveryOrderID: deliveryID, OrderID: 91, OrderType: "food", AssignedRiderID: ptrContractRider(supRider1)}
	pay, err := repo.QuotedRiderPayout(context.Background(), order)
	if err != nil || pay != 52.5 {
		t.Fatal("contract edit changed accepted pay", pay, err)
	}
	if err = repo.ConfigureContractPay(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	pay, err = repo.QuotedRiderPayout(context.Background(), order)
	if err != nil || pay != 52.5 {
		t.Fatal("rollout rollback changed frozen assignment pay", pay, err)
	}
	if _, err = db.Exec(`UPDATE delivery_orders SET delivery_status='cancelled' WHERE delivery_order_id=$1`, deliveryID); err != nil {
		t.Fatal(err)
	}
	var cents, id int64
	if err = db.QueryRow(`SELECT id,proposed_cents FROM rider_cancellation_pay_reviews`).Scan(&id, &cents); err != nil || cents != 2125 {
		t.Fatal(cents, err)
	}
	var balance float64
	if err = db.QueryRow(`SELECT COALESCE((SELECT balance FROM rider_wallet_balances WHERE rider_id=$1),0)`, supRider1).Scan(&balance); err != nil || balance != 0 {
		t.Fatal("cancellation auto-paid before review", balance, err)
	}
	encoded, _ := json.Marshal(map[string]any{"approve": true, "note": "Confirmed picked-up cancellation eligibility"})
	call("POST", fmt.Sprintf("/review/%d", id), string(encoded), 200)
	call("POST", fmt.Sprintf("/review/%d", id), string(encoded), 409)
	if err = db.QueryRow(`SELECT balance FROM rider_wallet_balances WHERE rider_id=$1`, supRider1).Scan(&balance); err != nil || balance != 21.25 {
		t.Fatal(balance, err)
	}
	call("POST", "/salary/"+supRider1, `{"period_start":"2026-08-01","note":"Approved August salary"}`, 201)
	call("POST", "/salary/"+supRider1, `{"period_start":"2026-08-01","note":"Duplicate period"}`, 409)
	call("POST", "/salary/"+supRider1, `{"period_start":"2026-08-15","note":"Overlapping period"}`, 409)
	if err = db.QueryRow(`SELECT balance FROM rider_wallet_balances WHERE rider_id=$1`, supRider1).Scan(&balance); err != nil || balance != 20021.25 {
		t.Fatal("salary mixed with per-order pay or credited twice", balance, err)
	}
}
func ptrContractRider(s string) *string { return &s }

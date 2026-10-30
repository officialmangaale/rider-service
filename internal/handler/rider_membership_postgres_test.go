package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
	"github.com/gin-gonic/gin"
)

func membershipDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testpg.Open(t)
	if err := testpg.ApplyLifecycle(db); err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("../../migrations/004_create_rider_notifications.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(ddl)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`ALTER TABLE users ADD COLUMN deleted_at timestamptz;INSERT INTO restaurants(restaurant_id,name) VALUES(1,'Kitchen One'),(2,'Kitchen Two');
 INSERT INTO users(id,primary_role,first_name) VALUES('d1000000-0000-4000-8000-000000000001','delivery_driver','Rider');`); err != nil {
		t.Fatal(err)
	}
	ddl, err = os.ReadFile("../testpg/testdata/137_rider_membership.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(ddl)); err != nil {
		t.Fatalf("membership migration: %v", err)
	}
	return db
}
func invite(t *testing.T, db *sql.DB, restaurant int) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`INSERT INTO restaurant_rider_invitations(restaurant_id,rider_id,invited_by) VALUES($1,$2,'owner') RETURNING invitation_id`, restaurant, supRider1).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestPermanentCodeInvitationConsentAndExclusivity(t *testing.T) {
	db := membershipDB(t)
	ctx := context.Background()
	var code string
	if err := db.QueryRow(`SELECT rider_code FROM rider_work_profiles WHERE rider_id=$1`, supRider1).Scan(&code); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE rider_work_profiles SET rider_code='CHANGED' WHERE rider_id=$1`, supRider1); err == nil {
		t.Fatal("permanent rider code changed")
	}
	if _, err := db.Exec(`INSERT INTO restaurant_riders(restaurant_id,rider_user_id)VALUES(1,$1)`, supRider1); err == nil {
		t.Fatal("direct link bypassed rider consent")
	}
	id := invite(t, db, 1)
	other := invite(t, db, 2)
	var linked int
	if err := db.QueryRow(`SELECT count(*) FROM restaurant_riders`).Scan(&linked); err != nil || linked != 0 {
		t.Fatal("invitation linked before consent", err)
	}
	var notice string
	if err := db.QueryRow(`SELECT body FROM rider_notifications WHERE rider_id=$1 ORDER BY created_at LIMIT 1`, supRider1).Scan(&notice); err != nil || notice != "Do you want to join Kitchen One restaurant?" {
		t.Fatal("invitation notification", notice, err)
	}
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,true)`, id, supRider1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,true)`, id, supRider1); err != nil {
		t.Fatal("accept retry not idempotent", err)
	}
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,true)`, other, supRider1); err == nil {
		t.Fatal("rider joined two restaurants")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := repository.NewDeliveryRepository(db).LockPlatformRider(ctx, tx, supRider1); err == nil {
		t.Fatal("hired rider accepted platform work")
	}
	var after string
	if err := db.QueryRow(`SELECT rider_code FROM rider_work_profiles WHERE rider_id=$1`, supRider1).Scan(&after); err != nil || after != code {
		t.Fatal("code changed after joining")
	}
}
func TestInvitationRequiresIdleRiderAndCorrectRecipient(t *testing.T) {
	db := membershipDB(t)
	id := invite(t, db, 1)
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,true)`, id, supRider2); err == nil {
		t.Fatal("another rider answered invitation")
	}
	if _, err := db.Exec(`INSERT INTO rider_availability(rider_id,is_online,is_available,current_order_id)VALUES($1,true,false,42)`, supRider1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,true)`, id, supRider1); err == nil {
		t.Fatal("rider joined while on trip")
	}
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,false)`, id, supRider1); err != nil {
		t.Fatal("decline should work on trip", err)
	}
}
func TestTypedDeliveryHistoryUsesActualWalletEarnings(t *testing.T) {
	db := membershipDB(t)
	_, err := db.Exec(`INSERT INTO delivery_orders(order_id,order_type,restaurant_id,customer_id,pickup_latitude,pickup_longitude,drop_latitude,drop_longitude,assigned_rider_id,delivery_status,restaurant_owned,restaurant_name)
 VALUES(7,'food',1,1,28,77,28.1,77.1,$1,'delivered',false,'Food'),(7,'grocery',1,1,28,77,28.1,77.1,$1,'delivered',false,'Shop'),(8,'food',1,1,28,77,28.1,77.1,$1,'delivered',true,'Own');`, supRider1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO rider_wallet_transactions(rider_id,order_id,order_type,transaction_type,amount,balance_before,balance_after)
 VALUES($1,7,'food','delivery_earning',45,0,45),($1,7,'grocery','delivery_earning',0,45,45);`, supRider1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO orders(order_id,restaurant_id,order_type,order_status,delivery_partner_id)VALUES(9,1,'DELIVERY','completed',$1)`, supRider1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO rider_earnings(rider_id,order_id,order_type,type,amount)VALUES($1,9,'food','delivery_fee',22)`, supRider1); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewRiderMembershipHandler(db)
	r.Use(func(c *gin.Context) { c.Set("user_id", supRider1) })
	r.GET("/history", h.History)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/history", bytes.NewReader(nil)))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			Items []struct {
				ID       string  `json:"id"`
				Earnings float64 `json:"earnings"`
				Own      bool    `json:"restaurant_owned"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil {
		t.Fatal(w.Body.String())
	}
	got := map[string]float64{}
	for _, i := range response.Data.Items {
		got[i.ID] = i.Earnings
	}
	if len(got) != 4 || got["food:7"] != 45 || got["grocery:7"] != 0 || got["food:8"] != 0 || got["food:9"] != 22 {
		t.Fatal(got)
	}
}

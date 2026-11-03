package handler

import (
	"context"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnerRemovalRequiresApplicationAndAuditedApproval(t *testing.T) {
	db := membershipDB(t)
	id := invite(t, db, 1)
	if _, err := db.Exec(`SELECT mangaale_answer_rider_invitation($1,$2,true)`, id, supRider1); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.QueryRow(`SELECT rider_code FROM rider_work_profiles WHERE rider_id=$1`, supRider1).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO orders(order_id,restaurant_id,order_status,assigned_rider_user_id)VALUES(81,1,'preparing',$1)`, supRider1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE restaurant_riders SET is_active=false WHERE rider_user_id=$1`, supRider1); err == nil {
		t.Fatal("owner removed rider mid-delivery")
	}
	if _, err := db.Exec(`UPDATE orders SET order_status='delivered' WHERE order_id=81`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rider_availability(rider_id,current_order_id)VALUES($1,81) ON CONFLICT(rider_id)DO UPDATE SET current_order_id=81`, supRider1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE restaurant_riders SET is_active=false WHERE rider_user_id=$1`, supRider1); err == nil {
		t.Fatal("removal ignored a live workload with a stale parent order")
	}
	if _, err := db.Exec(`UPDATE rider_availability SET current_order_id=NULL WHERE rider_id=$1`, supRider1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE restaurant_riders SET is_active=false WHERE rider_user_id=$1`, supRider1); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM rider_platform_access WHERE rider_id=$1`, supRider1).Scan(&state); err != nil || state != "reapply_required" {
		t.Fatal(state, err)
	}
	if _, err := db.Exec(`INSERT INTO rider_availability(rider_id,is_online,is_available)VALUES($1,true,true)`, supRider1); err == nil {
		t.Fatal("removed rider went online without approval")
	}
	lockAllowed := func(want bool) {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		err = repository.NewDeliveryRepository(db).LockPlatformRider(context.Background(), tx, supRider1)
		if (err == nil) != want {
			t.Fatalf("platform acceptance allowed=%t, want %t: %v", err == nil, want, err)
		}
	}
	lockAllowed(false)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", supRider1) })
	router.POST("/apply", NewRiderMembershipHandler(db).Reapply)
	admin := NewAdminRiderHandler(db)
	router.POST("/review/:riderId", admin.ReviewPlatformApplication)
	router.GET("/queue", admin.PlatformApplications)
	call := func(path, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	call("/apply", `{}`, 200)
	call("/apply", `{}`, 200)
	lockAllowed(false)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/queue", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Kitchen One") {
		t.Fatal(w.Code, w.Body.String())
	}
	call("/review/"+supRider1, `{"approve":true,"note":"Verified return to platform"}`, 200)
	call("/review/"+supRider1, `{"approve":true,"note":"Retry"}`, 409)
	lockAllowed(true)
	if _, err := db.Exec(`INSERT INTO rider_availability(rider_id,is_online,is_available)VALUES($1,true,true) ON CONFLICT(rider_id)DO UPDATE SET is_online=true,is_available=true`, supRider1); err != nil {
		t.Fatal(err)
	}
	var final string
	var count int
	if err := db.QueryRow(`SELECT rider_code FROM rider_work_profiles WHERE rider_id=$1`, supRider1).Scan(&final); err != nil || final != original {
		t.Fatal("code changed", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM rider_platform_access_audit WHERE rider_id=$1`, supRider1).Scan(&count); err != nil || count != 3 {
		t.Fatal("retry duplicated audit", count, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM rider_notifications WHERE rider_id=$1::uuid AND body LIKE '%can now go online%'`, supRider1).Scan(&count); err != nil || count != 1 {
		t.Fatal("approval notice", count, err)
	}
}

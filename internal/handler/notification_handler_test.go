package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/testpg"
)

const riderForTokens = "c6b46748-0000-4000-8000-000000000001"

func tokenRoutes(t *testing.T) (*gin.Engine, func() []string) {
	t.Helper()
	db := testpg.Open(t)
	if _, err := db.Exec(`CREATE TABLE notification_devices (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id TEXT NOT NULL, outlet_id TEXT NOT NULL,
		user_id TEXT NOT NULL, platform TEXT NOT NULL, push_token TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL);
		CREATE UNIQUE INDEX uq_notification_devices_token ON notification_devices (tenant_id, user_id, push_token);`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewNotificationRepository(db)
	h := NewNotificationHandler(service.NewNotificationService(repo))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", riderForTokens) })
	r.POST("/notifications/device-token", h.RegisterDeviceToken)
	r.DELETE("/notifications/device-token", h.UnregisterDeviceToken)

	stored := func() []string {
		rows, err := db.Query(`SELECT push_token FROM notification_devices WHERE user_id=$1 ORDER BY push_token`, riderForTokens)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var token string
			_ = rows.Scan(&token)
			out = append(out, token)
		}
		return out
	}
	return r, stored
}

func call(r *gin.Engine, method, body string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/notifications/device-token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code
}

func TestTheRiderAppsRegistrationBodyIsAccepted(t *testing.T) {
	r, stored := tokenRoutes(t)

	// This is the body the rider app has always sent. The endpoint used to
	// require `push_token` and answered it with 400, so no token was stored.
	if code := call(r, http.MethodPost, `{"platform":"android","device_token":"fcm-token-from-app"}`); code != http.StatusOK {
		t.Fatalf("app body: HTTP %d", code)
	}
	if got := stored(); len(got) != 1 || got[0] != "fcm-token-from-app" {
		t.Fatalf("stored %v", got)
	}

	// The documented key keeps working, and the app registers on every start.
	if code := call(r, http.MethodPost, `{"platform":"android","push_token":"fcm-token-from-app"}`); code != http.StatusOK {
		t.Fatalf("push_token body: HTTP %d", code)
	}
	if got := stored(); len(got) != 1 {
		t.Fatalf("a repeat registration duplicated the token: %v", got)
	}
}

func TestRegistrationRejectsWhatCannotBeAToken(t *testing.T) {
	r, stored := tokenRoutes(t)
	for name, body := range map[string]string{
		"no token":      `{"platform":"android"}`,
		"blank token":   `{"platform":"android","push_token":"  "}`,
		"no platform":   `{"device_token":"abc"}`,
		"bad platform":  `{"platform":"palmos","device_token":"abc"}`,
		"spaced token":  `{"platform":"ios","device_token":"a b"}`,
		"not json":      `nope`,
		"empty request": `{}`,
	} {
		if code := call(r, http.MethodPost, body); code != http.StatusBadRequest {
			t.Errorf("%s: HTTP %d, want 400", name, code)
		}
	}
	if got := stored(); len(got) != 0 {
		t.Fatalf("stored %v", got)
	}
}

func TestSignOutForgetsTheToken(t *testing.T) {
	r, stored := tokenRoutes(t)
	_ = call(r, http.MethodPost, `{"platform":"android","device_token":"tok"}`)

	if code := call(r, http.MethodDelete, `{"device_token":"tok"}`); code != http.StatusOK {
		t.Fatalf("HTTP %d", code)
	}
	if got := stored(); len(got) != 0 {
		t.Fatalf("token survived sign-out: %v", got)
	}
	// Retried after a lost response: still fine.
	if code := call(r, http.MethodDelete, `{"push_token":"tok"}`); code != http.StatusOK {
		t.Fatalf("repeat HTTP %d", code)
	}
	if code := call(r, http.MethodDelete, `{}`); code != http.StatusBadRequest {
		t.Fatalf("empty unregister: HTTP %d", code)
	}
}

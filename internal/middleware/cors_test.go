package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func corsHeader(t *testing.T, allowedEnv, origin string) string {
	t.Helper()
	t.Setenv("ALLOWED_ORIGINS", allowedEnv)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Header().Get("Access-Control-Allow-Origin")
}

func TestCORSRestrictsToConfiguredOrigins(t *testing.T) {
	const list = "https://admin-pannel.mangaale.com, https://food.mangaale.com"
	if got := corsHeader(t, list, "https://admin-pannel.mangaale.com"); got != "https://admin-pannel.mangaale.com" {
		t.Errorf("listed origin: got %q", got)
	}
	if got := corsHeader(t, list, "https://evil.example"); got != "" {
		t.Errorf("unlisted origin must get no CORS grant, got %q", got)
	}
	if got := corsHeader(t, list, ""); got != "" {
		t.Errorf("no Origin header (mobile app) gets no browser grant, got %q", got)
	}
}

func TestCORSUnsetKeepsTheOldOpenBehaviour(t *testing.T) {
	if got := corsHeader(t, "", "https://anything.example"); got != "*" {
		t.Errorf("unset ALLOWED_ORIGINS: got %q, want *", got)
	}
}

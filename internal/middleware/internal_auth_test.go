package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireInternalServiceToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/internal", RequireInternalServiceToken("secret"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/internal", nil)
	req.Header.Set("X-Internal-Service-Token", "wrong")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/internal", nil)
	req.Header.Set("X-Internal-Service-Token", "secret")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("valid token status = %d", resp.Code)
	}
}

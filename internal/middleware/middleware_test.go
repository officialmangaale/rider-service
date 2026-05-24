package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestRiderEndpointsAcceptSameUserServiceJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "test-secret"
	token := signedTestToken(t, secret)

	router := gin.New()
	protected := router.Group("/api/v1")
	protected.Use(AuthMiddleware(secret))
	protected.GET("/rider/profile", okHandler)
	protected.GET("/rider/availability", okHandler)
	protected.GET("/orders/active", okHandler)

	for _, path := range []string{"/api/v1/rider/profile", "/api/v1/rider/availability", "/api/v1/orders/active"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("%s rejected shared user-service JWT: status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
}

func okHandler(c *gin.Context) {
	if GetUserID(c) == "" {
		c.Status(http.StatusUnauthorized)
		return
	}
	c.Status(http.StatusOK)
}

func signedTestToken(t *testing.T, secret string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "0796784c-7d7c-42f3-9360-ec371a8da17a",
		"role":  "delivery_driver",
		"phone": "9876543210",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

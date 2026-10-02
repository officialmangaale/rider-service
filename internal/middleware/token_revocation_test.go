package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type fakeRevocation struct {
	revoked bool
	err     error
	gotUser string
	gotIat  time.Time
}

func (f *fakeRevocation) IsRevoked(_ context.Context, _, userID string, iat time.Time) (bool, error) {
	f.gotUser, f.gotIat = userID, iat
	return f.revoked, f.err
}

func authStatus(t *testing.T, secret string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "rider-1", "role": "delivery_driver",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/x", AuthMiddleware(secret), func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestAuthMiddlewareRejectsRevokedToken(t *testing.T) {
	t.Cleanup(func() { SetTokenRevocationChecker(nil) })

	f := &fakeRevocation{revoked: true}
	SetTokenRevocationChecker(f)
	if got := authStatus(t, "s"); got != http.StatusUnauthorized {
		t.Fatalf("revoked token: status = %d, want 401", got)
	}
	if f.gotUser != "rider-1" || f.gotIat.IsZero() {
		t.Fatalf("checker got user=%q iat=%v", f.gotUser, f.gotIat)
	}

	SetTokenRevocationChecker(&fakeRevocation{})
	if got := authStatus(t, "s"); got != http.StatusOK {
		t.Fatalf("valid token: status = %d, want 200", got)
	}

	// Fails open when revocation cannot be determined.
	SetTokenRevocationChecker(&fakeRevocation{revoked: true, err: errors.New("db down")})
	if got := authStatus(t, "s"); got != http.StatusOK {
		t.Fatalf("checker error: status = %d, want 200 (fail open)", got)
	}
}

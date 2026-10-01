package middleware

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenRevocationChecker reports whether a presented access token was revoked
// server-side (logout, account deletion). Satisfied by
// repository.TokenRevocationRepo; the revocation tables are written by
// user-service and shared through the common database.
type TokenRevocationChecker interface {
	IsRevoked(ctx context.Context, rawToken, userID string, issuedAt time.Time) (bool, error)
}

// Process-wide, installed once at startup, so AuthMiddleware's signature and
// its call sites stay unchanged. Nil (the default) disables the check.
var revocationChecker atomic.Pointer[TokenRevocationChecker]

// SetTokenRevocationChecker installs the checker; nil disables it.
func SetTokenRevocationChecker(checker TokenRevocationChecker) {
	if checker == nil {
		revocationChecker.Store(nil)
		return
	}
	revocationChecker.Store(&checker)
}

// TokenIsRevoked applies the installed checker. It fails OPEN, like
// restaurant-service: if revocation cannot be determined (database blip) the
// request proceeds rather than signing every rider out mid-delivery. Failures
// are logged so an outage is visible.
func TokenIsRevoked(ctx context.Context, rawToken, userID string, claims jwt.MapClaims) bool {
	stored := revocationChecker.Load()
	if stored == nil {
		return false
	}
	revoked, err := (*stored).IsRevoked(ctx, rawToken, userID, claimIssuedAt(claims))
	if err != nil {
		log.Printf("[AUTH] token revocation check failed - allowing request user_id=%s error=%v", userID, err)
		return false
	}
	return revoked
}

// claimIssuedAt reads iat; an unreadable value yields the zero time, which the
// cutoff comparison treats as "cannot judge" instead of rejecting.
func claimIssuedAt(claims jwt.MapClaims) time.Time {
	switch v := claims["iat"].(type) {
	case float64:
		if v > 0 {
			return time.Unix(int64(v), 0).UTC()
		}
	case int64:
		if v > 0 {
			return time.Unix(v, 0).UTC()
		}
	}
	return time.Time{}
}

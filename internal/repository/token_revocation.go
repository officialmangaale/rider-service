package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"
)

// TokenRevocationRepo is the read side of user-service's revocation tables:
//   - auth_token_revocations: individually revoked tokens (logout), keyed by a
//     SHA-256 hex hash of the raw token.
//   - auth_user_token_cutoffs: per-user instant before which every token is
//     dead (account deletion).
type TokenRevocationRepo struct {
	db *sql.DB
	// A positive table detection is cached; a negative one is not, so a
	// migration applied after startup is picked up without a restart.
	tablesPresent atomic.Bool
}

func NewTokenRevocationRepo(db *sql.DB) *TokenRevocationRepo {
	return &TokenRevocationRepo{db: db}
}

// HashAccessToken matches user-service/restaurant-service's denylist key.
func HashAccessToken(rawToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawToken)))
	return hex.EncodeToString(sum[:])
}

func (r *TokenRevocationRepo) detectTables(ctx context.Context) bool {
	if r.tablesPresent.Load() {
		return true
	}
	var present bool
	err := r.db.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='auth_token_revocations')
		AND
		EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='auth_user_token_cutoffs')`).Scan(&present)
	if err != nil || !present {
		return false
	}
	r.tablesPresent.Store(true)
	return true
}

// IsRevoked reports whether the token must be rejected. Errors mean "could not
// determine"; the caller decides how to degrade.
func (r *TokenRevocationRepo) IsRevoked(ctx context.Context, rawToken, userID string, issuedAt time.Time) (bool, error) {
	if r == nil || r.db == nil || strings.TrimSpace(rawToken) == "" {
		return false, nil
	}
	if !r.detectTables(ctx) {
		return false, nil
	}
	// Runs on every authenticated request: bound it tightly.
	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var tokenRevoked bool
	var cutoff sql.NullTime
	err := r.db.QueryRowContext(queryCtx, `
		SELECT
			EXISTS (SELECT 1 FROM auth_token_revocations WHERE token_hash = $1),
			(SELECT tokens_invalid_before FROM auth_user_token_cutoffs WHERE user_id = $2)
	`, HashAccessToken(rawToken), strings.TrimSpace(userID)).Scan(&tokenRevoked, &cutoff)
	if err != nil {
		return false, err
	}
	if tokenRevoked {
		return true, nil
	}
	// iat has one-second resolution, so compare at that resolution.
	if cutoff.Valid && !issuedAt.IsZero() &&
		!issuedAt.Truncate(time.Second).After(cutoff.Time.Truncate(time.Second)) {
		return true, nil
	}
	return false, nil
}

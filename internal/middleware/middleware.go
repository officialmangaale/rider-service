package middleware

import (
	"os"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/debug"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
)

// AuthMiddleware validates JWT tokens using the shared secret from user-service.
// Extracts "sub" (user UUID) and "role" from JWT claims.
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			dto.Unauthorized(c, "Authorization header required")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			dto.Unauthorized(c, "Invalid authorization format")
			c.Abort()
			return
		}

		tokenStr := parts[1]
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			dto.Unauthorized(c, "Invalid or expired token")
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			dto.Unauthorized(c, "Invalid token claims")
			c.Abort()
			return
		}

		// Extract "sub" — user UUID (string)
		sub, _ := claims["sub"].(string)
		if sub == "" {
			dto.Unauthorized(c, "Token missing sub claim")
			c.Abort()
			return
		}

		// Extract "role"
		role, _ := claims["role"].(string)

		// Extract "phone" if present
		phone, _ := claims["phone"].(string)

		// A signed, unexpired token can still have been revoked (logout,
		// account deletion). 401 so the app signs the rider out.
		if TokenIsRevoked(c.Request.Context(), tokenStr, sub, claims) {
			dto.Unauthorized(c, "Invalid or expired token")
			c.Abort()
			return
		}

		c.Set("user_id", sub)
		c.Set("user_role", role)
		c.Set("user_phone", phone)
		debug.Logf("auth ok endpoint=%s %s user_id=%s role=%s", c.Request.Method, c.FullPath(), sub, role)

		c.Next()
	}
}

// GetUserID extracts the user ID from the Gin context (set by AuthMiddleware).
func GetUserID(c *gin.Context) string {
	id, _ := c.Get("user_id")
	s, _ := id.(string)
	return s
}

// GetUserRole extracts the user role from the Gin context.
func GetUserRole(c *gin.Context) string {
	role, _ := c.Get("user_role")
	s, _ := role.(string)
	return s
}

// RequireAdmin gates a route group to callers whose JWT "role" claim is
// "admin". Must run after AuthMiddleware. rider-service has no granular
// permission tiers (unlike restaurant-service's roleRepo-backed
// RequirePermission) — that is a future module's scope; today "admin" is a
// single flat role.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GetUserRole(c) != "admin" {
			dto.Forbidden(c, "Admin access required")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CORSMiddleware sets CORS headers. When ALLOWED_ORIGINS (comma separated) is
// configured, only those browser origins are allowed; when it is unset every
// origin is allowed, as before, so a deployment without the setting keeps
// working. Mobile apps send no Origin header and are unaffected either way.
func CORSMiddleware() gin.HandlerFunc {
	allowed := parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		switch {
		case len(allowed) == 0:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "" && allowed[origin]:
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func parseAllowedOrigins(raw string) map[string]bool {
	out := map[string]bool{}
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out[o] = true
		}
	}
	return out
}

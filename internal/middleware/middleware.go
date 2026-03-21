package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

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

		c.Set("user_id", sub)
		c.Set("user_role", role)
		c.Set("user_phone", phone)

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

// CORSMiddleware adds permissive CORS headers.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

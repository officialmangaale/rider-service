package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireInternalServiceToken protects narrow service-to-service routes.
func RequireInternalServiceToken(expectedToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(expectedToken)
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "internal service authentication not configured"})
			return
		}
		provided := strings.TrimSpace(c.GetHeader("X-Internal-Service-Token"))
		if provided == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "missing X-Internal-Service-Token header"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid service token"})
			return
		}
		c.Next()
	}
}

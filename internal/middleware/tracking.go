package middleware

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"net/http"
)

// Tracking contains private delivery locations. Knowing an order number is
// not authorization. Customer tracking in restaurant-service is unchanged.
func TrackingToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" && c.Query("token") != "" {
			c.Request.Header.Set("Authorization", "Bearer "+c.Query("token"))
		}
	}
}

func TrackingAccess(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var allowed bool
		err := db.QueryRowContext(c.Request.Context(), `SELECT EXISTS(
            SELECT 1 FROM orders o LEFT JOIN restaurants r ON r.restaurant_id=o.restaurant_id
            WHERE o.order_id::text=$1 AND NOT o.is_deleted AND
              (o.customer_id=$2 OR o.metadata->>'customer_user_id'=$2 OR
               o.assigned_rider_user_id=$2 OR o.delivery_partner_id=$2 OR r.owner_auth_user_id::text=$2))`,
			c.Param("orderId"), GetUserID(c)).Scan(&allowed)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "Could not verify tracking access"})
			return
		}
		if !allowed || c.Query("order_type") == "grocery" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "Order not accessible"})
			return
		}
	}
}

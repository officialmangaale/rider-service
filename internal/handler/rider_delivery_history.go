package handler

import (
	"encoding/json"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/gin-gonic/gin"
)

const riderHistoryRows = `WITH history AS (
 SELECT d.order_id,COALESCE(d.order_type,'food') AS order_type,d.restaurant_name,d.customer_name,
 d.pickup_address,d.drop_address,d.delivery_status AS status,d.payment_mode,
 COALESCE(d.restaurant_owned,false) AS own,COALESCE(d.delivered_at,d.updated_at) AS ended_at,d.created_at,
 CASE WHEN d.delivered_at IS NOT NULL THEN greatest(0,round(extract(epoch FROM d.delivered_at-d.created_at)/60)) ELSE 0 END AS duration_minutes,
 round((6371*acos(least(1.0,greatest(-1.0,sin(radians(d.pickup_latitude))*sin(radians(d.drop_latitude))+
 cos(radians(d.pickup_latitude))*cos(radians(d.drop_latitude))*cos(radians(d.drop_longitude-d.pickup_longitude))))))::numeric,2) AS distance_km
 FROM delivery_orders d
 WHERE COALESCE(NULLIF(d.assigned_rider_id,''),d.rider_user_id)=$1
 AND d.delivery_status IN ('delivered','cancelled','failed') AND NOT COALESCE(d.is_deleted,false)
 UNION ALL
 SELECT o.order_id,'food',r.name,COALESCE(o.metadata->>'customer_name',''),r.street_address,o.delivery_address,
 lower(o.order_status::text),o.pay_by,COALESCE(o.metadata->>'delivery_mode','')='restaurant_own_rider',
 COALESCE(o.delivered_at,o.updated_at),o.created_at,0,0
 FROM orders o LEFT JOIN restaurants r ON r.restaurant_id=o.restaurant_id
 WHERE (o.delivery_partner_id=$1 OR o.assigned_rider_user_id=$1)
 AND upper(o.order_type::text)='DELIVERY' AND lower(o.order_status::text) IN ('delivered','completed','cancelled','failed','rejected')
 AND NOT COALESCE(o.is_deleted,false)
 AND NOT EXISTS(SELECT 1 FROM delivery_orders d WHERE d.order_type='food' AND d.order_id=o.order_id)
) `

// History uses delivery projections, so food/grocery ID collisions and own
// rider assignments cannot disappear behind the legacy delivery_partner_id.
// Earnings come from the posted wallet ledger, never the customer delivery fee.
func (h *RiderMembershipHandler) History(c *gin.Context) {
	var page dto.PaginationQuery
	_ = c.ShouldBindQuery(&page)
	page.Normalize()
	rider := middleware.GetUserID(c)
	var total int64
	if err := h.db.QueryRowContext(c.Request.Context(), riderHistoryRows+`SELECT count(*) FROM history`, rider).Scan(&total); err != nil {
		dto.InternalError(c, "History unavailable")
		return
	}
	rows, err := h.db.QueryContext(c.Request.Context(), riderHistoryRows+`SELECT jsonb_build_object(
 'id',h.order_type||':'||h.order_id::text,'order_id',h.order_id,'order_type',h.order_type,
 'restaurant_name',h.restaurant_name,'customer_name',h.customer_name,'pickup_address',h.pickup_address,'drop_address',h.drop_address,
 'order_status',h.status,'delivery_status',h.status,'payment_method',h.payment_mode,
 'restaurant_owned',h.own,'delivered_at',h.ended_at,'created_at',h.created_at,'distance_km',h.distance_km,'duration_minutes',h.duration_minutes,
 'earnings',CASE WHEN h.own THEN 0 ELSE COALESCE(
 (SELECT sum(w.amount) FROM rider_wallet_transactions w WHERE w.rider_id::text=$1 AND w.order_id=h.order_id AND w.order_type=h.order_type AND w.transaction_type='delivery_earning'),
 (SELECT sum(e.amount) FROM rider_earnings e WHERE e.rider_id::text=$1 AND e.order_id=h.order_id AND e.order_type=h.order_type AND NOT COALESCE(e.is_deleted,false)),0) END,
 'notes',CASE WHEN h.own THEN 'Paid directly by restaurant' ELSE 'Recorded platform delivery earnings' END
 ) FROM history h ORDER BY h.ended_at DESC,h.order_type,h.order_id DESC LIMIT $2 OFFSET $3`, rider, page.Limit, page.Offset())
	if err != nil {
		dto.InternalError(c, "History unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var raw []byte
		var item map[string]interface{}
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &item) != nil {
			dto.InternalError(c, "History unavailable")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		dto.InternalError(c, "History unavailable")
		return
	}
	dto.Paginated(c, items, page.Page, page.Limit, total)
}

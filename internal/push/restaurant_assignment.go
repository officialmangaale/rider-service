package push

import (
	"fmt"
	"strconv"
	"time"
)

type AssignmentNotice struct {
	OrderID        int
	RestaurantName string
}

func (n *Notifier) RestaurantOrderAssigned(riderID string, notice AssignmentNotice) {
	n.dispatch(riderID, "restaurant_assignment", func(token DeviceToken) Message {
		return Message{
			Token:        token.Token,
			Data:         map[string]string{"type": "RESTAURANT_ORDER_ASSIGNED", "order_id": strconv.Itoa(notice.OrderID), "assignment_type": "restaurant_owned"},
			TTL:          time.Hour,
			AndroidAlert: true,
			CollapseKey:  fmt.Sprintf("restaurant-assignment-%d", notice.OrderID),
			Alert:        &Alert{Title: "Restaurant order assigned", Body: fmt.Sprintf("%s assigned order #%d. Open the app for pickup and delivery details.", notice.RestaurantName, notice.OrderID)},
		}
	})
}

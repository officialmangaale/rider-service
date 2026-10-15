package push

import (
	"strconv"
	"time"
)

// ChatNotice is what a push may say about a customer's chat message.
//
// It carries the order number and a short preview of the text, never the
// customer's name, phone or address. A notification can appear on a lock
// screen; the rider sees the customer's details only inside the app.
type ChatNotice struct {
	OrderID   int
	MessageID int64
	Preview   string
}

// chatPushTTL is how long FCM may hold a chat push for an unreachable phone.
// A message about a delivery is useless long after the delivery.
const chatPushTTL = time.Hour

// ChatMessage pushes a chat message to every device the rider has registered.
// Like offers, it returns at once and a failure never affects the message,
// which is already stored and delivered over the socket.
func (n *Notifier) ChatMessage(riderID string, notice ChatNotice) {
	if n == nil {
		return
	}
	n.dispatch(riderID, "chat_message", func(token DeviceToken) Message {
		return buildChatMessage(token, notice)
	})
}

func buildChatMessage(token DeviceToken, notice ChatNotice) Message {
	order := strconv.Itoa(notice.OrderID)
	return Message{
		Token: token.Token,
		Data: map[string]string{
			"type":       "chat_message",
			"order_id":   order,
			"message_id": strconv.FormatInt(notice.MessageID, 10),
			"preview":    notice.Preview,
		},
		TTL:         chatPushTTL,
		CollapseKey: "chat-" + order,
		Alert: &Alert{
			Title: "Message about order #" + order,
			Body:  notice.Preview,
		},
	}
}

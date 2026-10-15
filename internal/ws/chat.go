package ws

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// chatKeyPrefix keys chat sockets apart from order-tracking sockets. It must
// match service.ChatOrderKey.
const chatKeyPrefix = "chat:"

// HandleOrderChatWS upgrades a customer's chat connection for one order.
//
// The caller must already have passed the chat participant check; this only
// registers the socket. It is deliberately a separate channel from order
// tracking: tracking is readable by the restaurant owner, a chat is not.
func (h *Hub) HandleOrderChatWS() gin.HandlerFunc {
	return func(c *gin.Context) {
		orderID := c.Param("orderId")
		if orderID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "order_id required"})
			return
		}
		conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[WS] Failed to upgrade chat connection: %v", err)
			return
		}
		key := chatKeyPrefix + orderID
		client := &Client{
			conn:     conn,
			hub:      h,
			channel:  key,
			entityID: key,
			isRider:  false,
			send:     make(chan []byte, 64),
			done:     make(chan struct{}),
		}
		h.addOrderClient(key, client)
		go client.writePump()
		go client.readPump()
	}
}

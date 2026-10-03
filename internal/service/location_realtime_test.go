package service

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestActiveRiderLocationBroadcastsToOrderTrackingSocket(t *testing.T) {
	svc, mock := newRedispatchService(t)
	mock.ExpectQuery("SELECT order_id").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}).AddRow(4242))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ws/tracking/orders/:orderId", svc.hub.HandleOrderTrackingWS())
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/tracking/orders/4242"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial tracking websocket: %v", err)
	}
	defer conn.Close()

	svc.NotifyRiderLocationUpdated(context.Background(), "rider-1", RiderLocationUpdate{
		Latitude:  28.45,
		Longitude: 77.02,
	})

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Type string `json:"type"`
		Data struct {
			RiderID   string  `json:"rider_id"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"data"`
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read tracking message: %v", err)
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("decode tracking message: %v", err)
	}
	if msg.Type != "RIDER_LOCATION_UPDATED" || msg.Data.RiderID != "rider-1" || msg.Data.Latitude != 28.45 || msg.Data.Longitude != 77.02 {
		t.Fatalf("unexpected tracking message: %s", string(raw))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInactiveRiderLocationDoesNotBroadcast(t *testing.T) {
	svc, mock := newRedispatchService(t)
	mock.ExpectQuery("SELECT order_id").
		WithArgs("rider-1").
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}))

	svc.NotifyRiderLocationUpdated(context.Background(), "rider-1", RiderLocationUpdate{
		Latitude:  28.45,
		Longitude: 77.02,
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

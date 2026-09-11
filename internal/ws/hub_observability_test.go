package ws

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dispatchtrace"
)

func captureDispatch(t *testing.T) *[]string {
	t.Helper()
	// Emit may run on several goroutines at once (concurrent accepts), so the
	// capture is locked like the real logger.
	var lines []string
	var mu sync.Mutex
	restore := dispatchtrace.SetOutput(func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(format, args...))
	})
	t.Cleanup(restore)
	return &lines
}

// An offer sent to a rider with no live socket used to vanish without a
// trace. The caller can now see that it reached nobody.
func TestSendToRiderCountReportsNoConnection(t *testing.T) {
	hub := NewHub()

	connections, enqueued := hub.SendToRiderCount("rider-1", WSMessage{Type: "DELIVERY_ORDER_REQUEST"})

	if connections != 0 || enqueued != 0 {
		t.Fatalf("got %d/%d, want 0/0", connections, enqueued)
	}
}

func TestSendToRiderCountReportsBackpressure(t *testing.T) {
	hub := NewHub()
	open := &Client{send: make(chan []byte, 1), isRider: true}
	full := &Client{send: make(chan []byte), isRider: true} // unbuffered: always full
	hub.addRiderClient("rider-1", open)
	hub.addRiderClient("rider-1", full)

	connections, enqueued := hub.SendToRiderCount("rider-1", WSMessage{Type: "DELIVERY_ORDER_REQUEST"})

	if connections != 2 || enqueued != 1 {
		t.Fatalf("got %d/%d, want 2/1", connections, enqueued)
	}
	if hub.RiderConnectionCount("rider-1") != 2 {
		t.Fatal("RiderConnectionCount mismatch")
	}
}

func rejectRequest(t *testing.T, url string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	lines := captureDispatch(t)
	router := gin.New()
	router.GET("/ws/rider", NewHub().HandleRiderWS("test-secret"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec, strings.Join(*lines, "\n")
}

// Refused sockets are logged with a reason, never with the token or URL.
func TestRejectedSocketsLogAReasonAndNeverTheToken(t *testing.T) {
	rec, logged := rejectRequest(t, "/ws/rider")
	if rec.Code != http.StatusUnauthorized || !strings.Contains(logged, "reason_code=token_missing") {
		t.Fatalf("missing-token rejection: %d %q", rec.Code, logged)
	}

	const token = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJyaWRlci0xIn0.not-a-real-signature"
	rec, logged = rejectRequest(t, "/ws/rider?token="+token)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(logged, "reason_code=token_invalid") {
		t.Fatalf("bad-token rejection: %d %q", rec.Code, logged)
	}
	for _, fragment := range []string{"eyJ", "not-a-real-signature", "token="} {
		if strings.Contains(logged, fragment) {
			t.Fatalf("token material %q reached the log: %q", fragment, logged)
		}
	}
}

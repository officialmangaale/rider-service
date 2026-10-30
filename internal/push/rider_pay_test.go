package push

import (
	"strings"
	"testing"
	"time"
)

func TestOfferPushCarriesQuotedPayIncludingZero(t *testing.T) {
	for _, pay := range []float64{0, 35.5} {
		offer := Offer{RequestID: 1, OrderID: 7, Amount: 250, RiderPayout: &pay, ExpiresAt: time.Now().Add(time.Minute)}
		msg := buildOfferMessage(DeviceToken{Token: "token", Platform: "ios"}, offer, time.Minute)
		if msg.Data["rider_payout"] == "" || msg.Alert == nil || !strings.Contains(msg.Alert.Body, "Your pay Rs") {
			t.Fatal(msg)
		}
	}
	msg := buildOfferMessage(DeviceToken{}, Offer{Amount: 250}, time.Minute)
	if _, ok := msg.Data["rider_payout"]; ok {
		t.Fatal("invented missing payout")
	}
}

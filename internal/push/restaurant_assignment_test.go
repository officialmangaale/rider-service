package push

import (
	"testing"
	"time"
)

func TestRestaurantAssignmentNotifiesOnlyAssignedRiderOnAndroidAndIOS(t *testing.T) {
	sender := &recordingSender{}
	tokens := &memoryTokens{tokens: map[string][]DeviceToken{
		"assigned": {{Token: "android", Platform: "android"}, {Token: "ios", Platform: "ios"}},
		"other":    {{Token: "unrelated", Platform: "android"}},
	}}
	notifier := NewNotifier(sender, tokens)
	notifier.RestaurantOrderAssigned("assigned", AssignmentNotice{OrderID: 81, RestaurantName: "Iconic"})
	notifier.Wait()
	messages := sender.messages()
	if len(messages) != 2 {
		t.Fatalf("got %d notifications", len(messages))
	}
	for _, message := range messages {
		if message.Token == "unrelated" || message.Data["type"] != "RESTAURANT_ORDER_ASSIGNED" || message.Data["order_id"] != "81" {
			t.Fatalf("wrong recipient/payload: %+v", message)
		}
		payload := buildFCMMessage(message, time.Now())
		android := payload["android"].(map[string]any)
		if _, ok := android["notification"]; !ok {
			t.Fatal("Android assignment was silent/data-only")
		}
		aps := payload["apns"].(map[string]any)["payload"].(map[string]any)["aps"].(map[string]any)
		if _, ok := aps["alert"]; !ok {
			t.Fatal("iOS assignment was silent")
		}
	}
}

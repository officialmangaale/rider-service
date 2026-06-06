package worker

import "testing"

func TestUnwrapSNSMessageAcceptsMinifiedEnvelope(t *testing.T) {
	body := `{"Type":"Notification","Message":"{\"event_type\":\"ORDER_PLACED\",\"order_id\":42}"}`
	unwrapped, ok := unwrapSNSMessage(body)
	if !ok {
		t.Fatal("expected SNS envelope to be unwrapped")
	}
	if unwrapped != `{"event_type":"ORDER_PLACED","order_id":42}` {
		t.Fatalf("unexpected unwrapped body: %s", unwrapped)
	}
}

func TestUnwrapSNSMessageLeavesDirectEventAlone(t *testing.T) {
	body := `{"event_type":"ORDER_PLACED","order_id":42}`
	unwrapped, ok := unwrapSNSMessage(body)
	if ok {
		t.Fatal("direct event must not be treated as an SNS envelope")
	}
	if unwrapped != body {
		t.Fatalf("direct event changed: %s", unwrapped)
	}
}

func TestIsDeliveryOrderTypeNormalizesCaseAndWhitespace(t *testing.T) {
	for _, orderType := range []string{"delivery", "DELIVERY", " Delivery "} {
		if !isDeliveryOrderType(orderType) {
			t.Fatalf("expected %q to be accepted", orderType)
		}
	}
	for _, orderType := range []string{"", "pickup", "dine_in"} {
		if isDeliveryOrderType(orderType) {
			t.Fatalf("expected %q to be rejected", orderType)
		}
	}
}

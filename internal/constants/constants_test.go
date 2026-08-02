package constants

import "testing"

func TestRiderAllowedTransitionsAreCanonicalAndOneStep(t *testing.T) {
	tests := []struct {
		name string
		from OrderStatus
		to   OrderStatus
		want bool
	}{
		{name: "pickup", from: OrderReady, to: OrderOutForDelivery, want: true},
		{name: "delivery", from: OrderOutForDelivery, to: OrderDelivered, want: true},
		{name: "cannot skip pickup", from: OrderReady, to: OrderDelivered, want: false},
		{name: "cannot cancel canonical order", from: OrderOutForDelivery, to: OrderCancelled, want: false},
		{name: "cannot move terminal order", from: OrderDelivered, to: OrderOutForDelivery, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsRiderAllowedTransition(test.from, test.to); got != test.want {
				t.Fatalf("IsRiderAllowedTransition(%q, %q) = %t; want %t", test.from, test.to, got, test.want)
			}
		})
	}
}

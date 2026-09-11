package service

import "testing"

// Only an explicitly restaurant-owned event is held for a live own rider.
// restaurant-service is the authority on that decision; this service
// dispatches everything else.

func TestExplicitRestaurantOwnedEventsAreReChecked(t *testing.T) {
	for _, mode := range []string{"restaurant_own_rider", "restaurant_owned", " Restaurant_Own_Rider "} {
		if !requiresOwnRiderCheck(mode) {
			t.Errorf("%q must be re-checked for a live own rider", mode)
		}
	}
}

// 94% of delivery orders carry an empty mode. They have always dispatched in
// practice, because the check they triggered failed on every call. Making that
// check succeed for them would have started holding orders at any restaurant
// with an online own rider, whether or not its owner expected to assign one.
func TestAnEmptyModeDispatchesWithoutAnOwnRiderCheck(t *testing.T) {
	for _, mode := range []string{"", "   "} {
		if requiresOwnRiderCheck(mode) {
			t.Errorf("an empty delivery mode (%q) must dispatch, not be held", mode)
		}
	}
}

// restaurant-service sends an explicit "platform" once it has decided no own
// rider is live. That decision must not be second-guessed here.
func TestPlatformModeIsNeverHeld(t *testing.T) {
	for _, mode := range []string{"platform", "PLATFORM", "something_new"} {
		if requiresOwnRiderCheck(mode) {
			t.Errorf("%q must dispatch", mode)
		}
	}
}

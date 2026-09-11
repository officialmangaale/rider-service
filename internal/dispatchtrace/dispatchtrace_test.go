package dispatchtrace

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func capture(t *testing.T) *[]string {
	t.Helper()
	var lines []string
	restore := SetOutput(func(format string, args ...interface{}) {
		lines = append(lines, fmt.Sprintf(format, args...))
	})
	t.Cleanup(restore)
	return &lines
}

func TestEmitWritesOneStableSearchableLine(t *testing.T) {
	lines := capture(t)

	Emit(EventEligibilityEvaluated, Fields{
		"result":          "error",
		"order_id":        13294,
		"rejected_counts": map[string]int{"rider_location_stale": 5, "rider_not_available": 0},
		"radius_km":       5.0,
		"expires_in":      30 * time.Second,
	})

	want := "[DISPATCH] event=dispatch.eligibility.evaluated expires_in=30.0s order_id=13294 radius_km=5.00 rejected_counts=rider_location_stale:5,rider_not_available:0 result=error"
	if len(*lines) != 1 || (*lines)[0] != want {
		t.Fatalf("got %q\nwant %q", *lines, want)
	}
}

// A value can never start a new line or a new field.
func TestEmitCannotBeInjected(t *testing.T) {
	lines := capture(t)

	Emit(EventConnectionRejected, Fields{"reason_code": "x\n[DISPATCH] event=forged rider_id=1 ok=true"})

	line := (*lines)[0]
	if strings.Contains(line, "\n") || strings.Count(line, "event=") != 1 || strings.Contains(line, " ok=") {
		t.Fatalf("value escaped its field: %q", line)
	}
}

func TestLongValuesAreTruncated(t *testing.T) {
	lines := capture(t)

	Emit(EventWriteFailed, Fields{"error": strings.Repeat("a", 1000)})

	if len((*lines)[0]) > 250 {
		t.Fatalf("line not bounded: %d bytes", len((*lines)[0]))
	}
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestTraceIsOffByDefault(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	if tr := LoadTrace(env(nil), now); tr.Covers(1, now) {
		t.Fatal("tracing must be off without configuration")
	}
	// A rider id alone is not enough: the expiry is mandatory.
	if tr := LoadTrace(env(map[string]string{"DISPATCH_TRACE_RIDER_ID": "r1"}), now); tr.Covers(1, now) {
		t.Fatal("tracing must need an expiry")
	}
	past := env(map[string]string{"DISPATCH_TRACE_RIDER_ID": "r1", "DISPATCH_TRACE_UNTIL": "2026-09-11T09:00:00Z"})
	if tr := LoadTrace(past, now); tr.Covers(1, now) {
		t.Fatal("an expired trace must be off")
	}
	garbage := env(map[string]string{"DISPATCH_TRACE_RIDER_ID": "r1", "DISPATCH_TRACE_UNTIL": "tomorrow"})
	if tr := LoadTrace(garbage, now); tr.Covers(1, now) {
		t.Fatal("an unparseable expiry must be off")
	}
}

func TestTraceTargetsAndExpires(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	tr := LoadTrace(env(map[string]string{
		"DISPATCH_TRACE_RIDER_ID": "r1",
		"DISPATCH_TRACE_ORDER_ID": "13294",
		"DISPATCH_TRACE_UNTIL":    "2026-09-11T12:00:00Z",
	}), now)

	if !tr.Covers(13294, now) || tr.Covers(13312, now) {
		t.Fatal("trace must cover only the target order")
	}
	if tr.Covers(13294, now.Add(3*time.Hour)) {
		t.Fatal("trace must stop at its expiry")
	}
}

func TestTraceLeftOnByMistakeIsClamped(t *testing.T) {
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	tr := LoadTrace(env(map[string]string{
		"DISPATCH_TRACE_RIDER_ID": "r1",
		"DISPATCH_TRACE_UNTIL":    "2030-01-01T00:00:00Z",
	}), now)
	if !tr.Until.Equal(now.Add(MaxTraceWindow)) {
		t.Fatalf("until = %s, want clamped to %s", tr.Until, now.Add(MaxTraceWindow))
	}
}

func TestRiderDecisionNamesTheFirstFailedFilter(t *testing.T) {
	age := int64(12035)
	km := 0.0023
	stale := RiderDecision{
		AvailabilityRow: true, Online: true, Available: true, Idle: true,
		LocationRow: true, LocationFresh: false, WithinRadius: true,
		LocationAgeSec: &age, DistanceKm: &km,
	}
	if got := stale.FirstFailure(); got != "rider_location_stale" {
		t.Fatalf("FirstFailure = %q", got)
	}
	f := stale.Fields()
	if f["eligible"] != false || f["location_age_s"] != int64(12035) || f["distance_km"] != 0.0 {
		t.Fatalf("unexpected fields %+v", f)
	}

	eligible := stale
	eligible.LocationFresh = true
	if eligible.FirstFailure() != "" || eligible.Fields()["eligible"] != true {
		t.Fatal("a rider passing every filter must be eligible")
	}
	if (RiderDecision{}).FirstFailure() != "rider_availability_missing" {
		t.Fatal("a rider with no availability row fails first on that")
	}
}

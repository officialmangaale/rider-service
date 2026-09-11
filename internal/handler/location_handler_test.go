package handler

import (
	"encoding/json"
	"testing"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/dto"
)

// The online foreground service sends extra diagnostics. None of them may
// cost the rider a location update, however malformed.
func TestLocationRequestAcceptsForegroundServiceFields(t *testing.T) {
	body := `{
		"latitude": 28.4595, "longitude": 77.0266,
		"accuracy_meters": 20, "heading": 120, "speed": 4.3,
		"recorded_at": "2026-09-11T06:30:00.123456Z",
		"source": "foreground_service", "app_state": "background", "sequence": 1042
	}`
	var req dto.UpdateLocationRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.Source != "foreground_service" || req.AppState != "background" || req.Sequence == nil || *req.Sequence != 1042 {
		t.Fatalf("unexpected request %+v", req)
	}
}

// recorded_at is a plain string so an unusual format is carried, not
// rejected; the server never uses it for freshness.
func TestLocationRequestToleratesAnUnparseableRecordedAt(t *testing.T) {
	var req dto.UpdateLocationRequest
	if err := json.Unmarshal([]byte(`{"latitude":1,"longitude":2,"recorded_at":"yesterday-ish"}`), &req); err != nil {
		t.Fatalf("an odd recorded_at must not fail the request: %v", err)
	}
}

// An old app build sends only coordinates and must behave as before.
func TestLocationRequestFromALegacyBuild(t *testing.T) {
	var req dto.UpdateLocationRequest
	if err := json.Unmarshal([]byte(`{"latitude":1,"longitude":2}`), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if describeLocationSource(req.Source) != "legacy" {
		t.Fatalf("missing source should log as legacy, got %q", describeLocationSource(req.Source))
	}
}

func TestDescribeLocationSourceCannotForgeLogLines(t *testing.T) {
	for input, want := range map[string]string{
		"foreground_service":                "foreground_service",
		"app":                               "app",
		"":                                  "legacy",
		"x\n[DELIVERY] forged":              "other",
		"Background":                        "other",
		"a_very_long_source_label_xxxxxxxx": "other",
	} {
		if got := describeLocationSource(input); got != want {
			t.Errorf("describeLocationSource(%q) = %q, want %q", input, got, want)
		}
	}
}

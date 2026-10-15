package maps

import (
	"math"
	"testing"
)

func TestPolylineRoundTripAndKnownVector(t *testing.T) {
	// Google's documented example polyline.
	got, err := DecodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@")
	if err != nil {
		t.Fatal(err)
	}
	want := []Coordinate{{38.5, -120.2}, {40.7, -120.95}, {43.252, -126.453}}
	if len(got) != len(want) {
		t.Fatalf("got %d points", len(got))
	}
	for i := range want {
		if math.Abs(got[i].Latitude-want[i].Latitude) > 1e-5 || math.Abs(got[i].Longitude-want[i].Longitude) > 1e-5 {
			t.Fatalf("point %d got %+v want %+v", i, got[i], want[i])
		}
	}
	if enc := EncodePolyline(want); enc != "_p~iF~ps|U_ulLnnqC_mqNvxq`@" {
		t.Fatalf("encode got %q", enc)
	}
}

func TestDecodePolylineRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "   ", "test-polyline", "_p~iF", "\x01\x02\x03\x04"} {
		if _, err := DecodePolyline(in); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}

func TestProjectOnPolylineProgressAndOffset(t *testing.T) {
	// A straight east-west line of about 1.1 km.
	route := []Coordinate{{30.70, 76.700}, {30.70, 76.705}, {30.70, 76.710}}
	mid, ok := ProjectOnPolyline(route, Coordinate{30.70, 76.705})
	if !ok || mid.OffsetMeters > 1 {
		t.Fatalf("midpoint should be on route: %+v ok=%v", mid, ok)
	}
	if f := mid.RemainingFraction(); f < 0.49 || f > 0.51 {
		t.Fatalf("remaining fraction at midpoint got %v", f)
	}
	start, _ := ProjectOnPolyline(route, Coordinate{30.70, 76.700})
	end, _ := ProjectOnPolyline(route, Coordinate{30.70, 76.710})
	if start.RemainingFraction() != 1 || end.RemainingFraction() != 0 {
		t.Fatalf("start/end fractions got %v/%v", start.RemainingFraction(), end.RemainingFraction())
	}
	// 0.001 degrees of latitude is about 111 m off the route.
	off, _ := ProjectOnPolyline(route, Coordinate{30.701, 76.705})
	if off.OffsetMeters < 100 || off.OffsetMeters > 120 {
		t.Fatalf("offset got %v want ~111", off.OffsetMeters)
	}
	if _, ok := ProjectOnPolyline(route[:1], route[0]); ok {
		t.Fatal("a single point is not a route")
	}
}

package maps

import (
	"errors"
	"math"
	"strings"
)

// maxPolylinePoints bounds decoding so a corrupt or hostile string cannot make
// the service allocate without limit. A drive route of tens of kilometres is a
// few hundred to a few thousand points.
const maxPolylinePoints = 20000

var errInvalidPolyline = errors.New("maps: invalid encoded polyline")

// DecodePolyline decodes a Google encoded polyline (precision 1e5).
func DecodePolyline(encoded string) ([]Coordinate, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errInvalidPolyline
	}
	points := make([]Coordinate, 0, len(encoded)/4)
	var index, lat, lng int
	next := func() (int, bool) {
		result, shift := 0, 0
		for {
			if index >= len(encoded) || shift > 30 {
				return 0, false
			}
			b := int(encoded[index]) - 63
			index++
			if b < 0 || b > 63 {
				return 0, false
			}
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		if result&1 != 0 {
			return ^(result >> 1), true
		}
		return result >> 1, true
	}
	for index < len(encoded) {
		dLat, ok := next()
		if !ok {
			return nil, errInvalidPolyline
		}
		dLng, ok := next()
		if !ok {
			return nil, errInvalidPolyline
		}
		lat += dLat
		lng += dLng
		c := Coordinate{Latitude: float64(lat) / 1e5, Longitude: float64(lng) / 1e5}
		if math.Abs(c.Latitude) > 90 || math.Abs(c.Longitude) > 180 {
			return nil, errInvalidPolyline
		}
		points = append(points, c)
		if len(points) > maxPolylinePoints {
			return nil, errInvalidPolyline
		}
	}
	if len(points) < 2 {
		return nil, errInvalidPolyline
	}
	return points, nil
}

// EncodePolyline is the inverse of DecodePolyline. It exists for tests and
// fixtures; production never builds route geometry itself.
func EncodePolyline(points []Coordinate) string {
	var b strings.Builder
	var prevLat, prevLng int
	put := func(v int) {
		v <<= 1
		if v < 0 {
			v = ^v
		}
		for v >= 0x20 {
			b.WriteByte(byte((0x20 | (v & 0x1f)) + 63))
			v >>= 5
		}
		b.WriteByte(byte(v + 63))
	}
	for _, p := range points {
		lat := int(math.Round(p.Latitude * 1e5))
		lng := int(math.Round(p.Longitude * 1e5))
		put(lat - prevLat)
		put(lng - prevLng)
		prevLat, prevLng = lat, lng
	}
	return b.String()
}

// DistanceMeters is the great-circle distance between two coordinates.
func DistanceMeters(a, b Coordinate) float64 {
	const earth = 6371000.0
	rad := math.Pi / 180
	dLat := (b.Latitude - a.Latitude) * rad
	dLng := (b.Longitude - a.Longitude) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(a.Latitude*rad)*math.Cos(b.Latitude*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earth * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}

// RouteProgress locates a point relative to a decoded route.
type RouteProgress struct {
	// OffsetMeters is the distance from the point to the nearest point on the
	// route. Large values mean the rider is no longer on this route.
	OffsetMeters float64
	// AlongMeters is the distance travelled along the route to that nearest
	// point; TotalMeters is the geometric length of the whole route.
	AlongMeters float64
	TotalMeters float64
}

// RemainingFraction is the share of the route still ahead, in [0,1].
func (p RouteProgress) RemainingFraction() float64 {
	if p.TotalMeters <= 0 {
		return 1
	}
	remaining := (p.TotalMeters - p.AlongMeters) / p.TotalMeters
	return math.Min(1, math.Max(0, remaining))
}

// ProjectOnPolyline finds the nearest point on route to p. The second result
// is false when route has fewer than two points or p is not a valid coordinate.
func ProjectOnPolyline(route []Coordinate, p Coordinate) (RouteProgress, bool) {
	if len(route) < 2 || !p.Valid() {
		return RouteProgress{}, false
	}
	var total float64
	best := RouteProgress{OffsetMeters: math.Inf(1)}
	for i := 0; i < len(route)-1; i++ {
		a, b := route[i], route[i+1]
		segLen := DistanceMeters(a, b)
		offset, t := segmentOffset(p, a, b)
		if offset < best.OffsetMeters {
			best.OffsetMeters = offset
			best.AlongMeters = total + t*segLen
		}
		total += segLen
	}
	best.TotalMeters = total
	return best, true
}

// segmentOffset returns the distance in metres from p to segment a-b and the
// fractional position (0..1) of the nearest point on it. A local equirectangular
// approximation is accurate to well under a metre at delivery distances.
func segmentOffset(p, a, b Coordinate) (float64, float64) {
	const latScale = 111320.0
	lngScale := latScale * math.Cos(p.Latitude*math.Pi/180)
	px, py := p.Longitude*lngScale, p.Latitude*latScale
	ax, ay := a.Longitude*lngScale, a.Latitude*latScale
	bx, by := b.Longitude*lngScale, b.Latitude*latScale
	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay), 0
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Min(1, math.Max(0, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy)), t
}

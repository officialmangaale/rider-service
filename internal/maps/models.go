// Package maps defines optional location providers. Existing dispatch and radius
// decisions do not depend on these interfaces.
package maps

import (
	"context"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/geo"
	"time"
)

type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (c Coordinate) Valid() bool { return geo.ValidCoordinate(c.Latitude, c.Longitude) }

type RouteOptions struct {
	TravelMode        string
	RoutingPreference string
	IncludePolyline   bool
}

func (o RouteOptions) normalized() (RouteOptions, error) {
	if o.TravelMode == "" {
		o.TravelMode = "DRIVE"
	}
	switch o.TravelMode {
	case "DRIVE", "TWO_WHEELER":
		if o.RoutingPreference == "" {
			o.RoutingPreference = "TRAFFIC_UNAWARE"
		}
		if o.RoutingPreference != "TRAFFIC_UNAWARE" && o.RoutingPreference != "TRAFFIC_AWARE" && o.RoutingPreference != "TRAFFIC_AWARE_OPTIMAL" {
			return o, &RouteError{Code: InvalidRequest}
		}
	case "WALK", "BICYCLE":
		if o.RoutingPreference != "" {
			return o, &RouteError{Code: InvalidRequest}
		}
	default:
		return o, &RouteError{Code: InvalidRequest}
	}
	return o, nil
}

type RouteRequest struct {
	Origin, Destination Coordinate
	Options             RouteOptions
}
type RouteLeg struct {
	DistanceMeters        int64
	DurationSeconds       float64
	StaticDurationSeconds float64
}
type RouteResult struct {
	RouteLeg
	Legs                   []RouteLeg
	Polyline               string
	TrafficDurationSeconds *float64
	Provider               string
	CalculatedAt           time.Time
}
type RouteMatrixRequest struct {
	Origins, Destinations []Coordinate
	Options               RouteOptions
}
type RouteMatrixElement struct {
	OriginIndex, DestinationIndex int
	Route                         *RouteResult
	Error                         *RouteError
}
type RouteMatrixResult struct {
	Elements     []RouteMatrixElement
	Provider     string
	CalculatedAt time.Time
}

type ErrorCode string

const (
	NotConfigured      ErrorCode = "NOT_CONFIGURED"
	ProviderDisabled   ErrorCode = "PROVIDER_DISABLED"
	InvalidCoordinates ErrorCode = "INVALID_COORDINATES"
	InvalidRequest     ErrorCode = "INVALID_REQUEST"
	NoRoute            ErrorCode = "NO_ROUTE"
	Timeout            ErrorCode = "TIMEOUT"
	Cancelled          ErrorCode = "CANCELLED"
	RateLimited        ErrorCode = "RATE_LIMITED"
	UpstreamError      ErrorCode = "UPSTREAM_ERROR"
	InvalidResponse    ErrorCode = "INVALID_RESPONSE"
)

// No raw response body, URL, key or upstream error message crosses this boundary.
type RouteError struct {
	Code       ErrorCode
	StatusCode int
}

func (e *RouteError) Error() string { return "maps provider: " + string(e.Code) }
func (e *RouteError) Is(target error) bool {
	return (e.Code == Timeout && target == context.DeadlineExceeded) || (e.Code == Cancelled && target == context.Canceled)
}

type RouteProvider interface {
	GetRoute(context.Context, RouteRequest) (RouteResult, error)
	GetRouteMatrix(context.Context, RouteMatrixRequest) (RouteMatrixResult, error)
}

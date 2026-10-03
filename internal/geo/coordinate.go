// Package geo contains the coordinate semantics shared by uploads and optional providers.
package geo

import "math"

func ValidCoordinate(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsInf(latitude, 0) && latitude >= -90 && latitude <= 90 &&
		!math.IsNaN(longitude) && !math.IsInf(longitude, 0) && longitude >= -180 && longitude <= 180 &&
		!(latitude == 0 && longitude == 0)
}

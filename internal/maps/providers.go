package maps

import "context"

// SessionToken is opaque, per-interaction and must not be reused across users.
type PlaceSearch struct {
	Query, SessionToken string
	Bias                *Coordinate
}
type Place struct {
	ID, DisplayName, Address string
	Coordinate               *Coordinate
}
type PlacesProvider interface {
	SearchPlaces(context.Context, PlaceSearch) ([]Place, error)
	GetPlaceDetails(context.Context, string, string) (Place, error)
}
type GeocodingResult struct {
	Coordinate       Coordinate
	FormattedAddress string
}
type GeocodingProvider interface {
	ForwardGeocode(context.Context, string) ([]GeocodingResult, error)
	ReverseGeocode(context.Context, Coordinate) ([]GeocodingResult, error)
}

// DisabledAddressProvider is the only production implementation in Phase 2.
// Test doubles implement the interfaces; fabricated addresses never enter production.
type DisabledAddressProvider struct{}

func (DisabledAddressProvider) SearchPlaces(context.Context, PlaceSearch) ([]Place, error) {
	return nil, &RouteError{Code: ProviderDisabled}
}
func (DisabledAddressProvider) GetPlaceDetails(context.Context, string, string) (Place, error) {
	return Place{}, &RouteError{Code: ProviderDisabled}
}
func (DisabledAddressProvider) ForwardGeocode(context.Context, string) ([]GeocodingResult, error) {
	return nil, &RouteError{Code: ProviderDisabled}
}
func (DisabledAddressProvider) ReverseGeocode(context.Context, Coordinate) ([]GeocodingResult, error) {
	return nil, &RouteError{Code: ProviderDisabled}
}

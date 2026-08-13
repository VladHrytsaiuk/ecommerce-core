package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

// LocationService owns provider selection and input validation for checkout
// delivery selectors. Provider HTTP calls remain in the selected adapter.
type LocationService struct{ carriers *Registry }

func NewLocationService(carriers *Registry) *LocationService {
	return &LocationService{carriers: carriers}
}

func (s *LocationService) Areas(ctx context.Context, provider string) ([]domain.Area, error) {
	locations, err := s.locations(provider)
	if err != nil {
		return nil, err
	}
	return locations.ListAreas(ctx)
}

func (s *LocationService) Cities(ctx context.Context, provider, areaID string) ([]domain.City, error) {
	areaID = strings.TrimSpace(areaID)
	if !validReference(areaID) {
		return nil, fmt.Errorf("%w: area_id", domain.ErrInvalidLocationQuery)
	}
	locations, err := s.locations(provider)
	if err != nil {
		return nil, err
	}
	return locations.ListCities(ctx, areaID)
}

func (s *LocationService) ServicePoints(ctx context.Context, provider string, query domain.ServicePointQuery) (domain.ServicePointPage, error) {
	query.CityID = strings.TrimSpace(query.CityID)
	query.Kind = strings.ToLower(strings.TrimSpace(query.Kind))
	if !validReference(query.CityID) || query.Page < 1 || query.Limit < 1 || query.Limit > 100 || !validKind(query.Kind) {
		return domain.ServicePointPage{}, fmt.Errorf("%w: service point query", domain.ErrInvalidLocationQuery)
	}
	locations, err := s.locations(provider)
	if err != nil {
		return domain.ServicePointPage{}, err
	}
	return locations.ListServicePoints(ctx, query)
}

func (s *LocationService) locations(provider string) (domain.LocationProvider, error) {
	if s == nil || s.carriers == nil {
		return nil, domain.ErrLocationProviderUnavailable
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	locations, ok := s.carriers.Locations(provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", domain.ErrLocationProviderUnavailable, provider)
	}
	return locations, nil
}

func validReference(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	return !strings.ContainsAny(value, "\x00\r\n")
}

func validKind(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "branch", "postomat", "cargo":
		return true
	default:
		return false
	}
}

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	sharedCache "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

const (
	areaAndCityCacheTTL  = 24 * time.Hour
	servicePointCacheTTL = 6 * time.Hour
)

// CachedLocationProvider is a fail-open read decorator: cache failures never
// make a carrier selector unavailable. Redis is only an optimisation; the
// provider remains authoritative. singleflight prevents a cache stampede.
type CachedLocationProvider struct {
	providerCode string
	delegate     domain.LocationProvider
	cache        sharedCache.Service
	group        singleflight.Group
}

func NewCachedLocationProvider(providerCode string, delegate domain.LocationProvider, cache sharedCache.Service) *CachedLocationProvider {
	return &CachedLocationProvider{providerCode: strings.ToLower(strings.TrimSpace(providerCode)), delegate: delegate, cache: cache}
}

func (p *CachedLocationProvider) ListAreas(ctx context.Context) ([]domain.Area, error) {
	key := p.key("areas")
	var areas []domain.Area
	if p.get(ctx, key, &areas) {
		return areas, nil
	}
	value, err, _ := p.group.Do(key, func() (any, error) {
		var cached []domain.Area
		if p.get(ctx, key, &cached) {
			return cached, nil
		}
		result, err := p.delegate.ListAreas(ctx)
		if err != nil {
			return nil, err
		}
		p.set(ctx, key, result, areaAndCityCacheTTL)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]domain.Area), nil
}

func (p *CachedLocationProvider) ListCities(ctx context.Context, areaID string) ([]domain.City, error) {
	key := p.key("cities", areaID)
	var cities []domain.City
	if p.get(ctx, key, &cities) {
		return cities, nil
	}
	value, err, _ := p.group.Do(key, func() (any, error) {
		var cached []domain.City
		if p.get(ctx, key, &cached) {
			return cached, nil
		}
		result, err := p.delegate.ListCities(ctx, areaID)
		if err != nil {
			return nil, err
		}
		p.set(ctx, key, result, areaAndCityCacheTTL)
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]domain.City), nil
}

func (p *CachedLocationProvider) ListServicePoints(ctx context.Context, query domain.ServicePointQuery) (domain.ServicePointPage, error) {
	key := p.key("service-points", query.CityID, query.Kind, fmt.Sprintf("%d", query.Page), fmt.Sprintf("%d", query.Limit))
	var page domain.ServicePointPage
	if p.get(ctx, key, &page) {
		return page, nil
	}
	value, err, _ := p.group.Do(key, func() (any, error) {
		var cached domain.ServicePointPage
		if p.get(ctx, key, &cached) {
			return cached, nil
		}
		result, err := p.delegate.ListServicePoints(ctx, query)
		if err != nil {
			return domain.ServicePointPage{}, err
		}
		p.set(ctx, key, result, servicePointCacheTTL)
		return result, nil
	})
	if err != nil {
		return domain.ServicePointPage{}, err
	}
	return value.(domain.ServicePointPage), nil
}

func (p *CachedLocationProvider) key(kind string, parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "delivery:locations:v1:" + p.providerCode + ":" + kind + ":" + hex.EncodeToString(h.Sum(nil))
}
func (p *CachedLocationProvider) get(ctx context.Context, key string, target any) bool {
	if p.cache == nil {
		return false
	}
	raw, err := p.cache.Get(ctx, key)
	return err == nil && json.Unmarshal(raw, target) == nil
}
func (p *CachedLocationProvider) set(ctx context.Context, key string, value any, ttl time.Duration) {
	if p.cache == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err == nil {
		_ = p.cache.Set(ctx, key, raw, ttl)
	}
}

var _ domain.LocationProvider = (*CachedLocationProvider)(nil)

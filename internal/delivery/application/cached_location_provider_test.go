package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	sharedCache "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

func TestCachedLocationProviderCoalescesConcurrentCacheMisses(t *testing.T) {
	provider := &slowLocations{entered: make(chan struct{}), release: make(chan struct{})}
	decorator := NewCachedLocationProvider("fake", provider, sharedCache.NewNoOpService())
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = decorator.ListAreas(context.Background()) }()
	}
	<-provider.entered
	// entered only says the first call is inside the provider; it says nothing
	// about the second having reached the flight it is meant to join. Releasing
	// immediately let the first finish before the second arrived, which starts a
	// second flight and two upstream calls — rare alone, reproducible under the
	// load of the full suite. The margin is scheduling slack, not a guarantee:
	// the property is real, it just cannot be observed from outside the
	// singleflight group.
	time.Sleep(100 * time.Millisecond)
	close(provider.release)
	wg.Wait()
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d, want the second caller to join the first flight", provider.calls)
	}
}

func TestCachedLocationProviderNoOpDoesNotPersist(t *testing.T) {
	provider := &slowLocations{release: make(chan struct{})}
	close(provider.release)
	decorator := NewCachedLocationProvider("fake", provider, sharedCache.NewNoOpService())
	if _, err := decorator.ListAreas(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := decorator.ListAreas(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls=%d, want 2", provider.calls)
	}
}

type slowLocations struct {
	calls            int
	entered, release chan struct{}
}

func (p *slowLocations) ListAreas(context.Context) ([]domain.Area, error) {
	p.calls++
	if p.entered != nil {
		select {
		case p.entered <- struct{}{}:
		default:
		}
	}
	<-p.release
	return []domain.Area{{ID: "a", Name: "A"}}, nil
}
func (*slowLocations) ListCities(context.Context, string) ([]domain.City, error) { return nil, nil }
func (*slowLocations) ListServicePoints(context.Context, domain.ServicePointQuery) (domain.ServicePointPage, error) {
	return domain.ServicePointPage{}, nil
}

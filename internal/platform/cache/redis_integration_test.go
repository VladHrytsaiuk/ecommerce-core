package cache_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	platformCache "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/cache"
	platformRedis "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/redis"
)

func TestRedisCacheAndRateLimiterIntegration(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
			AutoRemove:   true,
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start Redis container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Redis host: %v", err)
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatalf("Redis port: %v", err)
	}
	client, err := platformRedis.Connect(ctx, fmt.Sprintf("redis://%s:%s/0", host, port.Port()))
	if err != nil {
		t.Fatalf("connect Redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	adapter := platformCache.NewRedisAdapter(client.Raw(), "test:")
	if err := adapter.Set(ctx, "catalog:one", []byte("one"), time.Minute); err != nil {
		t.Fatalf("cache Set: %v", err)
	}
	if err := adapter.Set(ctx, "catalog:two", []byte("two"), time.Minute); err != nil {
		t.Fatalf("cache Set: %v", err)
	}
	if err := adapter.Set(ctx, "other", []byte("other"), time.Minute); err != nil {
		t.Fatalf("cache Set: %v", err)
	}
	if err := adapter.DeleteByPrefix(ctx, "catalog:"); err != nil {
		t.Fatalf("DeleteByPrefix: %v", err)
	}
	if _, err := adapter.Get(ctx, "catalog:one"); err == nil {
		t.Fatal("deleted cache entry was still returned")
	}
	if value, err := adapter.Get(ctx, "other"); err != nil || string(value) != "other" {
		t.Fatalf("unrelated entry = %q, %v", value, err)
	}

	limiter := platformRedis.NewFixedWindowLimiter(client.Raw())
	var wg sync.WaitGroup
	allowed := make(chan bool, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, callErr := limiter.Allow(ctx, "test:ratelimit:login", 5, time.Minute)
			if callErr != nil {
				t.Errorf("Allow: %v", callErr)
				return
			}
			allowed <- decision.Allowed
		}()
	}
	wg.Wait()
	close(allowed)
	count := 0
	for wasAllowed := range allowed {
		if wasAllowed {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("allowed concurrent requests = %d, want 5", count)
	}
}

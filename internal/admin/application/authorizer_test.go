package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

func TestAuthorizerUsesVersionedCacheAndReloadsAfterVersionChange(t *testing.T) {
	userID := uuid.New()
	repository := &accessRepositoryFake{
		state:       domain.AuthorizationState{UserID: userID, Version: 1},
		permissions: []string{"catalog:write"},
	}
	cacheService := newMemoryCache()
	authorizer, err := NewAuthorizer(repository, cacheService, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if err := authorizer.Require(context.Background(), userID, "catalog:write"); err != nil {
		t.Fatalf("first Require() error = %v", err)
	}
	if repository.permissionReads != 1 || cacheService.sets != 1 {
		t.Fatalf("first request reads=%d sets=%d, want cache miss then DB", repository.permissionReads, cacheService.sets)
	}
	if err := authorizer.Require(context.Background(), userID, "catalog:write"); err != nil {
		t.Fatalf("second Require() error = %v", err)
	}
	if repository.permissionReads != 1 {
		t.Fatalf("second request permission reads=%d, want cache hit", repository.permissionReads)
	}

	// Role/permission administration increments the version in the same DB
	// transaction. The old cache key is now unreachable without deleting it.
	repository.state.Version = 2
	repository.permissions = []string{"orders:read"}
	err = authorizer.Require(context.Background(), userID, "catalog:write")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("Require() after version change error = %v, want ErrPermissionDenied", err)
	}
	if repository.permissionReads != 2 || cacheService.sets != 2 {
		t.Fatalf("version change reads=%d sets=%d, want second cache miss", repository.permissionReads, cacheService.sets)
	}
}

type accessRepositoryFake struct {
	state           domain.AuthorizationState
	permissions     []string
	permissionReads int
}

func (r *accessRepositoryFake) FindAuthorizationState(context.Context, uuid.UUID) (domain.AuthorizationState, error) {
	return r.state, nil
}

func (r *accessRepositoryFake) ListPermissions(context.Context, uuid.UUID) ([]string, error) {
	r.permissionReads++
	return append([]string(nil), r.permissions...), nil
}

type memoryCache struct {
	mu     sync.Mutex
	values map[string][]byte
	sets   int
}

func newMemoryCache() *memoryCache { return &memoryCache{values: make(map[string][]byte)} }

func (c *memoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = append([]byte(nil), value...)
	c.sets++
	return nil
}
func (c *memoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	return append([]byte(nil), value...), nil
}
func (c *memoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, key)
	return nil
}
func (c *memoryCache) DeleteByPrefix(context.Context, string) error { return nil }

var _ domain.AccessRepository = (*accessRepositoryFake)(nil)
var _ cache.Service = (*memoryCache)(nil)

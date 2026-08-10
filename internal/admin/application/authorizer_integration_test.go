package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	adminApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

func TestAuthorizerIntegrationCachesPostgresPermissionsByAuthorizationVersion(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("admin_rbac_test"),
		postgresContainer.WithUsername("admin"),
		postgresContainer.WithPassword("admin"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("PostgreSQL connection string: %v", err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	if err := applyRBACSchema(db); err != nil {
		t.Fatalf("apply RBAC schema: %v", err)
	}

	userID, roleID, writePermissionID, readPermissionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, query := range []struct {
		statement string
		args      []any
	}{
		{"INSERT INTO users (id) VALUES (?)", []any{userID}},
		{"INSERT INTO admin_users (user_id, authorization_version) VALUES (?, 1)", []any{userID}},
		{"INSERT INTO roles (id, code, name) VALUES (?, 'catalog-editor', 'Catalog editor')", []any{roleID}},
		{"INSERT INTO permissions (id, code, resource, action) VALUES (?, 'catalog:write', 'catalog', 'write')", []any{writePermissionID}},
		{"INSERT INTO permissions (id, code, resource, action) VALUES (?, 'orders:read', 'orders', 'read')", []any{readPermissionID}},
		{"INSERT INTO admin_user_roles (user_id, role_id) VALUES (?, ?)", []any{userID, roleID}},
		{"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", []any{roleID, writePermissionID}},
	} {
		if err := db.Exec(query.statement, query.args...).Error; err != nil {
			t.Fatalf("seed RBAC: %v", err)
		}
	}

	cacheService := newIntegrationMemoryCache()
	repository := &countingRepository{AccessRepository: adminPostgres.NewRepository(db)}
	authorizer, err := adminApplication.NewAuthorizer(repository, cacheService, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Require(ctx, userID, "catalog:write"); err != nil {
		t.Fatalf("cache miss Require() error = %v", err)
	}
	if repository.permissionReads != 1 || cacheService.sets != 1 {
		t.Fatalf("cache miss reads=%d sets=%d, want 1/1", repository.permissionReads, cacheService.sets)
	}
	if err := authorizer.Require(ctx, userID, "catalog:write"); err != nil {
		t.Fatalf("cache hit Require() error = %v", err)
	}
	if repository.permissionReads != 1 {
		t.Fatalf("cache hit permission reads=%d, want 1", repository.permissionReads)
	}

	if err := db.Exec("DELETE FROM role_permissions WHERE role_id = ?", roleID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", roleID, readPermissionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE admin_users SET authorization_version = 2 WHERE user_id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Require(ctx, userID, "catalog:write"); !errors.Is(err, adminDomain.ErrPermissionDenied) {
		t.Fatalf("Require() after version increment = %v, want ErrPermissionDenied", err)
	}
	if repository.permissionReads != 2 {
		t.Fatalf("versioned cache permission reads=%d, want 2", repository.permissionReads)
	}
}

func applyRBACSchema(db *gorm.DB) error {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE users (id UUID PRIMARY KEY);").Error; err != nil {
		return err
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("locate RBAC migration")
	}
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../migrations/modules/admin/000001_init_rbac.up.sql"))
	if err != nil {
		return err
	}
	return db.Exec(string(migration)).Error
}

type countingRepository struct {
	adminDomain.AccessRepository
	permissionReads int
}

func (r *countingRepository) ListPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	r.permissionReads++
	return r.AccessRepository.ListPermissions(ctx, userID)
}

type integrationMemoryCache struct {
	mu     sync.Mutex
	values map[string][]byte
	sets   int
}

func newIntegrationMemoryCache() *integrationMemoryCache {
	return &integrationMemoryCache{values: make(map[string][]byte)}
}
func (c *integrationMemoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = append([]byte(nil), value...)
	c.sets++
	return nil
}
func (c *integrationMemoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	return append([]byte(nil), value...), nil
}
func (c *integrationMemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, key)
	return nil
}
func (c *integrationMemoryCache) DeleteByPrefix(context.Context, string) error { return nil }

var _ adminDomain.AccessRepository = (*countingRepository)(nil)
var _ cache.Service = (*integrationMemoryCache)(nil)

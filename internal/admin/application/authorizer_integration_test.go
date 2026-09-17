//go:build integration

package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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

// TestSuperAdminHoldsEveryGuardedPermission applies the admin migrations in
// order and checks that a super_admin can actually pass every permission gate
// the routes declare.
//
// The unit-level guard reads the SQL text and proves each permission is
// inserted somewhere. This proves the grants land too: four permissions were
// missing entirely, and a fifth could just as easily be created and never
// granted to any role, which fails the same way at request time.
func TestSuperAdminHoldsEveryGuardedPermission(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("rbac_seed_test"),
		postgresContainer.WithUsername("admin"),
		postgresContainer.WithPassword("admin"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyAdminMigrations(db); err != nil {
		t.Fatal(err)
	}

	var ungranted []string
	if err := db.Raw(`
		SELECT p.code FROM permissions p
		WHERE NOT EXISTS (
			SELECT 1 FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			WHERE rp.permission_id = p.id AND r.code = 'super_admin'
		)
		ORDER BY p.code`).Scan(&ungranted).Error; err != nil {
		t.Fatal(err)
	}
	if len(ungranted) > 0 {
		t.Fatalf("super_admin holds no grant for %v; every check of those denies", ungranted)
	}

	for _, code := range []string{"legal:write", "privacy:write", "support:read", "support:write", "video:write"} {
		var seeded int64
		if err := db.Raw(`SELECT COUNT(*) FROM permissions WHERE code = ?`, code).Scan(&seeded).Error; err != nil {
			t.Fatal(err)
		}
		if seeded != 1 {
			t.Fatalf("permission %q rows = %d, want exactly one", code, seeded)
		}
	}
}

// applyAdminMigrations runs the whole admin directory in order, so a later
// migration that alters what an earlier one seeded is reflected here.
func applyAdminMigrations(db *gorm.DB) error {
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE IF NOT EXISTS users (id UUID PRIMARY KEY);").Error; err != nil {
		return err
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("locate admin migrations")
	}
	root := filepath.Join(filepath.Dir(file), "../../..")
	// The audit-log migration references the core outbox, which this focused
	// fixture does not otherwise need.
	outbox, err := os.ReadFile(filepath.Join(root, "migrations/core/000005_add_event_outbox_and_order_contacts.up.sql"))
	if err != nil {
		return err
	}
	if index := strings.Index(string(outbox), "-- Order contact"); index >= 0 {
		outbox = outbox[:index]
	}
	if err := db.Exec(string(outbox)).Error; err != nil {
		return fmt.Errorf("apply core outbox: %w", err)
	}
	dir := filepath.Join(root, "migrations/modules/admin")
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		return readErr
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := db.Exec(string(raw)).Error; err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// TestRevokingAccessInvalidatesTheCachedPermissionSet covers the window the
// cache key was supposed to close and did not.
//
// The set is cached under admin_users.authorization_version. Only two CLI
// commands ever moved that number, and both of them grant — so a revocation,
// which has no API and is made directly in SQL, left the key unchanged and the
// old permissions in force until the TTL expired.
func TestRevokingAccessInvalidatesTheCachedPermissionSet(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("rbac_invalidation_test"),
		postgresContainer.WithUsername("admin"),
		postgresContainer.WithPassword("admin"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyAdminMigrations(db); err != nil {
		t.Fatal(err)
	}

	// catalog:write is seeded by the admin migrations, so this reuses it
	// rather than inserting a duplicate code.
	var permissionID uuid.UUID
	if err := db.Raw(`SELECT id FROM permissions WHERE code = 'catalog:write'`).Row().Scan(&permissionID); err != nil {
		t.Fatal(err)
	}
	userID, roleID := uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id) VALUES (?)", []any{userID}},
		{"INSERT INTO admin_users (user_id, authorization_version) VALUES (?, 1)", []any{userID}},
		{"INSERT INTO roles (id, code, name) VALUES (?, 'editor', 'Editor')", []any{roleID}},
		{"INSERT INTO admin_user_roles (user_id, role_id) VALUES (?, ?)", []any{userID, roleID}},
		{"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", []any{roleID, permissionID}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}

	// A long TTL, so anything that still works does so because the key
	// changed and not because the entry expired.
	authorizer, err := adminApplication.NewAuthorizer(adminPostgres.NewRepository(db), newRememberingCache(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := authorizer.Require(ctx, userID, "catalog:write"); err != nil {
		t.Fatalf("Require() before revocation = %v, want the granted permission", err)
	}

	for name, revoke := range map[string]string{
		"permission removed from the role": "DELETE FROM role_permissions WHERE role_id = ?",
		"role removed from the user":       "DELETE FROM admin_user_roles WHERE role_id = ?",
	} {
		t.Run(name, func(t *testing.T) {
			// Each subtest starts from a granted, cached state.
			if err := db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT DO NOTHING", roleID, permissionID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO admin_user_roles (user_id, role_id) VALUES (?, ?) ON CONFLICT DO NOTHING", userID, roleID).Error; err != nil {
				t.Fatal(err)
			}
			if err := authorizer.Require(ctx, userID, "catalog:write"); err != nil {
				t.Fatalf("Require() after regranting = %v", err)
			}

			if err := db.Exec(revoke, roleID).Error; err != nil {
				t.Fatal(err)
			}
			if err := authorizer.Require(ctx, userID, "catalog:write"); !errors.Is(err, adminDomain.ErrPermissionDenied) {
				t.Fatalf("Require() after revocation = %v, want it denied immediately rather than after the cache TTL", err)
			}
		})
	}
}

// rememberingCache actually stores what it is given, unlike the no-op used
// elsewhere in this file. Without a cache that remembers, a test of cache
// invalidation proves nothing: every lookup would be fresh.
type rememberingCache struct {
	mu      sync.Mutex
	entries map[string][]byte
}

func newRememberingCache() *rememberingCache {
	return &rememberingCache{entries: map[string][]byte{}}
}

func (c *rememberingCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = append([]byte(nil), value...)
	return nil
}

func (c *rememberingCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.entries[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	return append([]byte(nil), value...), nil
}

func (c *rememberingCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	return nil
}

func (c *rememberingCache) DeleteByPrefix(_ context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
	return nil
}

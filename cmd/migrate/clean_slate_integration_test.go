//go:build integration

package main

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	catalogPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/repository/postgres"
	localeApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/application"
	localePostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/repository/postgres"
)

// TestCleanSlateSchema proves the only migration path supported by the active
// application: a blank database receives core first and enabled modules next.
// It also proves the normalized three-locale Catalog path against PostgreSQL.
func TestCleanSlateSchema(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx,
		"postgres:16-alpine",
		containerPostgres.WithDatabase("ecommerce_core_test"),
		containerPostgres.WithUsername("ecommerce_core"),
		containerPostgres.WithPassword("ecommerce_core"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL container: %v", err)
	}
	t.Cleanup(func() {
		if terminateErr := container.Terminate(ctx); terminateErr != nil {
			t.Errorf("terminate PostgreSQL container: %v", terminateErr)
		}
	})

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get PostgreSQL connection string: %v", err)
	}
	root := repositoryRoot(t)
	if err := runMigrations(root, databaseURL, []string{"inventory", "sync"}, "up"); err != nil {
		t.Fatalf("migrate clean-slate schema: %v", err)
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open migrated PostgreSQL database: %v", err)
	}
	assertTablesExist(t, db,
		"locales", "categories", "category_translations", "products", "product_translations", "product_variants",
		"payment_webhook_events",
		"warehouses", "stock_items", "inventory_reservations", "schema_migrations", "schema_migrations_module_inventory",
		"sync_outbox", "sync_external_entity_state", "sync_cursors", "schema_migrations_module_sync",
	)

	localeService := localeApp.NewService(localePostgres.NewRepository(db))
	if err := localeService.Synchronize(ctx, []string{"es", "en", "ca"}, "es"); err != nil {
		t.Fatalf("synchronize configured locales: %v", err)
	}
	categoryService := catalogApp.NewCategoryService(catalogPostgres.NewCategoryRepository(db), []string{"es", "en", "ca"})
	category := &catalogDomain.Category{Translations: []catalogDomain.CategoryTranslation{
		{Locale: "es", Name: "Cuidado", Slug: "cuidado"},
		{Locale: "en", Name: "Skincare", Slug: "skincare"},
		{Locale: "ca", Name: "Cura", Slug: "cura"},
	}}
	if err := categoryService.Create(ctx, category); err != nil {
		t.Fatalf("create three-locale category: %v", err)
	}
	productService := catalogApp.NewProductService(catalogPostgres.NewProductRepository(db), []string{"es", "en", "ca"})
	product := &catalogDomain.Product{CategoryID: &category.ID, Status: "active", Translations: []catalogDomain.ProductTranslation{
		{Locale: "es", Name: "Crema", Slug: "crema"},
		{Locale: "en", Name: "Cream", Slug: "cream"},
		{Locale: "ca", Name: "Crema", Slug: "crema-ca"},
	}}
	if err := productService.Create(ctx, product); err != nil {
		t.Fatalf("create three-locale product: %v", err)
	}

	storedCategory, err := categoryService.FindBySlug(ctx, "en", "skincare")
	if err != nil || len(storedCategory.Translations) != 3 {
		t.Fatalf("read category translations = (%+v, %v), want three translations", storedCategory, err)
	}
	storedProduct, err := productService.FindBySlug(ctx, "ca", "crema-ca")
	if err != nil || len(storedProduct.Translations) != 3 {
		t.Fatalf("read product translations = (%+v, %v), want three translations", storedProduct, err)
	}
	if err := runMigrations(root, databaseURL, []string{"inventory", "sync"}, "down"); err != nil {
		t.Fatalf("rollback clean-slate schema: %v", err)
	}
	assertTablesAbsent(t, db, "locales", "products", "product_variants", "warehouses", "stock_items", "inventory_reservations", "sync_outbox", "sync_external_entity_state", "sync_cursors")
}

func assertTablesExist(t *testing.T, db *gorm.DB, tables ...string) {
	t.Helper()
	for _, table := range tables {
		var exists bool
		if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`, table).Scan(&exists).Error; err != nil {
			t.Fatalf("check table %q: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected migrated table %q", table)
		}
	}
}

func assertTablesAbsent(t *testing.T, db *gorm.DB, tables ...string) {
	t.Helper()
	for _, table := range tables {
		var exists bool
		if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`, table).Scan(&exists).Error; err != nil {
			t.Fatalf("check table %q: %v", table, err)
		}
		if exists {
			t.Fatalf("expected rolled-back table %q to be absent", table)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

//go:build integration

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	reviewsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/repository/postgres"
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
	if err := runMigrations(root, databaseURL, []string{"admin", "availability_notifications", "catalog", "comparison", "consent", "customers", "inventory", "reports", "returns", "reviews", "support", "sync", "user_profiles", "wishlist"}, "up"); err != nil {
		t.Fatalf("migrate clean-slate schema: %v", err)
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open migrated PostgreSQL database: %v", err)
	}
	assertTablesExist(t, db,
		"locales", "categories", "category_translations", "products", "product_translations", "product_variants",
		"payment_checkout_attempts", "payment_webhook_events", "order_delivery_details", "delivery_jobs", "user_oauth_identities", "oauth_authorization_attempts",
		"warehouses", "stock_items", "inventory_reservations", "schema_migrations", "schema_migrations_module_inventory",
		"sync_outbox", "sync_external_entity_state", "sync_cursors", "schema_migrations_module_sync",
		"user_profiles", "schema_migrations_module_user_profiles",
		"customer_profiles", "customer_addresses", "schema_migrations_module_customers",
		"product_media", "product_options", "product_option_values", "variant_option_values", "schema_migrations_module_catalog",
		"wishlist_items", "schema_migrations_module_wishlist",
		"comparison_lists", "comparison_items", "schema_migrations_module_comparison",
		"reviews", "product_review_ratings", "schema_migrations_module_reviews",
		"roles", "permissions", "report_processed_events", "report_daily_sales", "report_daily_product_sales", "report_daily_funnel", "schema_migrations_module_reports",
		"return_requests", "return_items", "return_status_history", "return_restock_operations", "schema_migrations_module_returns",
		"stock_subscriptions", "schema_migrations_module_availability_notifications",
		"support_tickets", "support_messages", "schema_migrations_module_support",
		"legal_documents", "customer_consents", "privacy_requests", "schema_migrations_module_consent",
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
	assertProductOptionMatrix(t, ctx, productService, catalogApp.NewProductOptionsService(catalogPostgres.NewOptionsRepository(db), "EUR"), product.ID)

	storedCategory, err := categoryService.FindBySlug(ctx, "en", "skincare")
	if err != nil || len(storedCategory.Translations) != 3 {
		t.Fatalf("read category translations = (%+v, %v), want three translations", storedCategory, err)
	}
	storedProduct, err := productService.FindBySlug(ctx, "ca", "crema-ca")
	if err != nil || len(storedProduct.Translations) != 3 {
		t.Fatalf("read product translations = (%+v, %v), want three translations", storedProduct, err)
	}
	assertConcurrentReviewProjection(t, db, product.ID)
	if err := runMigrations(root, databaseURL, []string{"admin", "availability_notifications", "catalog", "comparison", "consent", "customers", "inventory", "reports", "returns", "reviews", "support", "sync", "user_profiles", "wishlist"}, "down"); err != nil {
		t.Fatalf("rollback clean-slate schema: %v", err)
	}
	assertTablesAbsent(t, db, "locales", "products", "product_variants", "user_oauth_identities", "oauth_authorization_attempts", "user_profiles", "customer_profiles", "customer_addresses", "product_media", "wishlist_items", "comparison_lists", "comparison_items", "reviews", "product_review_ratings", "roles", "permissions", "report_processed_events", "report_daily_sales", "report_daily_product_sales", "report_daily_funnel", "return_requests", "return_items", "return_status_history", "return_restock_operations", "stock_subscriptions", "warehouses", "stock_items", "inventory_reservations", "sync_outbox", "sync_external_entity_state", "sync_cursors")
}

func assertProductOptionMatrix(t *testing.T, ctx context.Context, products *catalogApp.ProductService, options *catalogApp.ProductOptionsService, productID uuid.UUID) {
	t.Helper()
	color := &catalogDomain.ProductOption{ProductID: productID, Name: "Color", Values: []catalogDomain.ProductOptionValue{{Value: "Red"}, {Value: "Blue"}}}
	size := &catalogDomain.ProductOption{ProductID: productID, Name: "Size", Values: []catalogDomain.ProductOptionValue{{Value: "S"}, {Value: "M"}}}
	if err := options.CreateProductOption(ctx, color); err != nil {
		t.Fatalf("create color option: %v", err)
	}
	if err := options.CreateProductOption(ctx, size); err != nil {
		t.Fatalf("create size option: %v", err)
	}
	for _, colorValue := range color.Values {
		for _, sizeValue := range size.Values {
			price, err := money.NewMoney(1299, "EUR")
			if err != nil {
				t.Fatal(err)
			}
			variant := &catalogDomain.ProductVariant{ProductID: productID, Price: price}
			if err := options.CreateVariant(ctx, variant, []uuid.UUID{colorValue.ID, sizeValue.ID}); err != nil {
				t.Fatalf("create matrix variant: %v", err)
			}
		}
	}
	stored, err := products.FindByID(ctx, productID)
	if err != nil || len(stored.Options) != 2 || len(stored.Variants) != 4 {
		t.Fatalf("hydrate option matrix = (%+v, %v)", stored, err)
	}
	for _, variant := range stored.Variants {
		if len(variant.OptionValues) != 2 {
			t.Fatalf("variant %s has %d option values", variant.ID, len(variant.OptionValues))
		}
	}
}

func assertConcurrentReviewProjection(t *testing.T, db *gorm.DB, productID uuid.UUID) {
	t.Helper()
	userIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for index, userID := range userIDs {
		if err := db.Exec(`INSERT INTO users (id, email, role, status) VALUES (?, ?, 'customer', 'active')`, userID, fmt.Sprintf("reviewer%d@example.test", index+1)).Error; err != nil {
			t.Fatalf("create review user: %v", err)
		}
	}
	repository := reviewsPostgres.NewRepository(db)
	first, err := repository.Create(context.Background(), reviewsDomain.CreateCommand{ProductID: productID, UserID: userIDs[0], Rating: 5, Comment: "excellent"})
	if err != nil {
		t.Fatalf("create first review: %v", err)
	}
	second, err := repository.Create(context.Background(), reviewsDomain.CreateCommand{ProductID: productID, UserID: userIDs[1], Rating: 3, Comment: "good"})
	if err != nil {
		t.Fatalf("create second review: %v", err)
	}

	start := make(chan struct{})
	errorsByUpdate := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, reviewID := range []uuid.UUID{first.ID, second.ID} {
		waitGroup.Add(1)
		go func(id uuid.UUID) {
			defer waitGroup.Done()
			<-start
			_, updateErr := repository.SetStatus(context.Background(), id, reviewsDomain.StatusApproved)
			errorsByUpdate <- updateErr
		}(reviewID)
	}
	close(start)
	waitGroup.Wait()
	close(errorsByUpdate)
	for updateErr := range errorsByUpdate {
		if updateErr != nil {
			t.Fatalf("concurrent review moderation: %v", updateErr)
		}
	}
	rating, err := repository.RatingForProduct(context.Background(), productID)
	if err != nil || rating == nil || rating.ReviewCount != 2 || rating.AverageHundredths != 400 {
		t.Fatalf("rating projection = (%+v, %v), want count 2 and average 400", rating, err)
	}
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

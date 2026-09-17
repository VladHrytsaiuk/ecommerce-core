//go:build integration

package app

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
)

// .env.example is the configuration every new deployment starts from, and
// nothing exercised it. It shipped ENABLED_MODULES=inventory, which migrated
// cleanly, booted cleanly, and then failed every product read on product_media
// — a table only the catalog migrations create, for a module name
// ENABLED_MODULES did not even accept.
//
// This test takes the module list from that file rather than repeating it, so
// editing .env.example into a store that cannot serve a product fails here
// instead of in somebody's first deployment.
func TestTheShippedDefaultConfigurationServesAProduct(t *testing.T) {
	modules := shippedModules(t)
	db := freshDatabaseFor(t, modules)

	application, err := Bootstrap(testConfig(modules), storeConfigFor(t, modules), db, testTokenMaker(t))
	if err != nil {
		t.Fatalf("Bootstrap(%v) error = %v", modules, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = application.StopContext(ctx)
	})

	ctx := context.Background()
	productID := "11111111-1111-4111-8111-111111111111"
	if err := db.Exec(`INSERT INTO products (id, status) VALUES (?, 'active')`, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_translations (id, product_id, locale, name, slug) VALUES (gen_random_uuid(), ?, ?, 'Крем', 'krem')`,
		productID, testStoreLocale).Error; err != nil {
		t.Fatal(err)
	}

	// The two reads the storefront makes: a listing and a product page. Both
	// preload media, which is what used to fail.
	products, total, err := application.CatalogProductService.ListProducts(ctx, testStoreLocale, 1, 10)
	if err != nil {
		t.Fatalf("ListProducts() error = %v", err)
	}
	if len(products) != 1 || total != 1 {
		t.Fatalf("ListProducts() = %d of %d, want the one seeded product", len(products), total)
	}
	product, err := application.CatalogProductService.FindBySlug(ctx, testStoreLocale, "krem")
	if err != nil {
		t.Fatalf("FindBySlug() error = %v", err)
	}
	if product == nil || len(product.Translations) == 0 {
		t.Fatalf("FindBySlug() = %+v, want the seeded translation", product)
	}
}

// The store also has to be fillable and sellable, not only readable.
//
// DEFAULT_WAREHOUSE_ID named a warehouse nothing created — not a migration, not
// an endpoint, not a CLI command — so the first stock adjustment and every
// checkout reservation failed on a foreign key to a row that did not exist. A
// store following the documented setup could neither take goods in nor sell
// them, and nothing at startup said so.
func TestTheShippedDefaultConfigurationTakesStockAndHoldsItForACustomer(t *testing.T) {
	modules := shippedModules(t)
	db := freshDatabaseFor(t, modules)

	application, err := Bootstrap(testConfig(modules), storeConfigFor(t, modules), db, testTokenMaker(t))
	if err != nil {
		t.Fatalf("Bootstrap(%v) error = %v", modules, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = application.StopContext(ctx)
	})
	ctx := context.Background()

	warehouseID := application.StoreConfig.DefaultWarehouseID
	var active bool
	if err := db.Raw(`SELECT is_active FROM warehouses WHERE id = ?`, warehouseID).Scan(&active).Error; err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatalf("DEFAULT_WAREHOUSE_ID %s names no active warehouse; every reservation fails on the foreign key", warehouseID)
	}

	variantID := seedSellableVariant(t, db)

	// What the admin catalog facade does when a product is created with stock.
	if err := application.InventoryService.Adjust(ctx, variantID, warehouseID, 5); err != nil {
		t.Fatalf("Adjust() error = %v", err)
	}

	// And what checkout does for every cart, reserving against that same stock.
	prepared, err := application.CheckoutService.PreparePayment(ctx, checkoutDomain.PrepareRequest{
		CheckoutID: uuid.New(),
		Locale:     testStoreLocale,
		Lines:      []checkoutDomain.Line{{VariantID: variantID, WarehouseID: warehouseID, Quantity: 2}},
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("PreparePayment() error = %v", err)
	}
	if len(prepared.ReservationIDs) != 1 || len(prepared.Items) != 1 {
		t.Fatalf("prepared %d reservations and %d items, want one of each", len(prepared.ReservationIDs), len(prepared.Items))
	}
	if prepared.Total.Amount() <= 0 {
		t.Fatalf("prepared total = %d, want the seeded price", prepared.Total.Amount())
	}

	var reserved int
	if err := db.Raw(`SELECT quantity_reserved FROM stock_items WHERE variant_id = ? AND warehouse_id = ?`, variantID, warehouseID).Scan(&reserved).Error; err != nil {
		t.Fatal(err)
	}
	if reserved != 2 {
		t.Fatalf("quantity_reserved = %d, want the two units checkout is holding", reserved)
	}
}

// The README's next three steps are create-owner, grant-superadmin, then the
// admin API. All three need the admin module: its migrations create the RBAC
// tables grant-superadmin writes to, and without it no /api/v1/admin route is
// registered at all. The shipped module list has to carry it.
func TestTheShippedDefaultConfigurationCanBeAdministered(t *testing.T) {
	modules := shippedModules(t)
	db := freshDatabaseFor(t, modules)

	application, err := Bootstrap(testConfig(modules), storeConfigFor(t, modules), db, testTokenMaker(t))
	if err != nil {
		t.Fatalf("Bootstrap(%v) error = %v", modules, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = application.StopContext(ctx)
	})

	// The tables `cli grant-superadmin` writes to.
	for _, table := range []string{"roles", "permissions", "admin_users", "admin_user_roles"} {
		var exists bool
		if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`, table).Scan(&exists).Error; err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("%s does not exist; cli grant-superadmin cannot run on the shipped configuration", table)
		}
	}
	var superAdmin bool
	if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM roles WHERE code = 'super_admin')`).Scan(&superAdmin).Error; err != nil {
		t.Fatal(err)
	}
	if !superAdmin {
		t.Fatal("the super_admin role is not seeded; grant-superadmin has no role to grant")
	}

	// And the routes the step after that calls.
	if application.AdminAuthorizer == nil {
		t.Fatal("no admin authorizer; every /api/v1/admin route answers 503")
	}
	if application.CatalogAdminFacade == nil {
		t.Fatal("no catalog admin facade; the store cannot be filled through the API it documents")
	}
}

// seedSellableVariant writes the minimum the checkout snapshot reads: an active
// product with a translation for the store's locale, and an active priced
// variant.
func seedSellableVariant(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	productID, variantID := uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO products (id, status) VALUES (?, 'active')`, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_translations (id, product_id, locale, name, slug) VALUES (gen_random_uuid(), ?, ?, 'Крем', 'krem-sellable')`,
		productID, testStoreLocale).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_variants (id, product_id, sku, status, price_amount, currency)
		VALUES (?, ?, ?, 'active', 1299, 'UAH')`, variantID, productID, "SKU-"+variantID.String()).Error; err != nil {
		t.Fatal(err)
	}
	return variantID
}

// shippedModules reads ENABLED_MODULES out of .env.example.
func shippedModules(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repositoryRootForBootstrap(), ".env.example")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		value, found := strings.CutPrefix(line, "ENABLED_MODULES=")
		if !found {
			continue
		}
		modules := make([]string, 0, 4)
		for _, module := range strings.Split(value, ",") {
			if module = strings.TrimSpace(module); module != "" {
				modules = append(modules, module)
			}
		}
		if len(modules) == 0 {
			t.Fatalf("%s sets ENABLED_MODULES to nothing", path)
		}
		return modules
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("%s has no ENABLED_MODULES line", path)
	return nil
}

// freshDatabaseFor migrates core plus exactly the modules given, the way
// cmd/migrate plans them: a module that owns no schema has no directory and is
// skipped. It deliberately does not reuse the package's shared database, which
// carries every module's tables and so could not show this failure at all.
func freshDatabaseFor(t *testing.T, modules []string) *gorm.DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("shipped_default"),
		containerPostgres.WithUsername("shipped"),
		containerPostgres.WithPassword("shipped"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	root := repositoryRootForBootstrap()
	if err := migrateBootstrapDir(filepath.Join(root, "migrations", "core"), dsn, "schema_migrations"); err != nil {
		t.Fatal(err)
	}
	for _, module := range modules {
		dir := filepath.Join(root, "migrations", "modules", module)
		if _, statErr := os.Stat(dir); statErr != nil {
			continue
		}
		if err := migrateBootstrapDir(dir, dsn, "schema_migrations_module_"+module); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

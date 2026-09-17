//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

func TestUpdateVariantReplacesEveryMutableField(t *testing.T) {
	repository, db := newCatalogTestRepository(t)
	ctx := context.Background()
	variantID := seedVariant(t, db, "active", 1000)

	updated, err := repository.UpdateVariant(ctx, variantID, domain.UpdateVariantCommand{
		SKU: "CREAM-100", Barcode: "5901234", Status: "active",
		Price: mustTestMoney(t, 2500, "EUR"), WeightGrams: 400,
	})
	if err != nil {
		t.Fatalf("UpdateVariant() error = %v", err)
	}
	if updated.SKU != "CREAM-100" || updated.Barcode != "5901234" || updated.Price.Amount() != 2500 || updated.WeightGrams != 400 {
		t.Fatalf("UpdateVariant() = %+v", updated)
	}
}

func TestUpdateVariantClearsAnEmptyOptionalIdentifier(t *testing.T) {
	repository, db := newCatalogTestRepository(t)
	ctx := context.Background()
	variantID := seedVariant(t, db, "active", 1000)

	// SKU and barcode are UNIQUE, so blanking one must write NULL: an empty
	// string would collide with every other variant that has none.
	if _, err := repository.UpdateVariant(ctx, variantID, domain.UpdateVariantCommand{
		SKU: "", Barcode: "", Status: "active", Price: mustTestMoney(t, 1000, "EUR"),
	}); err != nil {
		t.Fatalf("UpdateVariant() error = %v", err)
	}
	second := seedVariant(t, db, "active", 1000)
	if _, err := repository.UpdateVariant(ctx, second, domain.UpdateVariantCommand{
		SKU: "", Barcode: "", Status: "active", Price: mustTestMoney(t, 1000, "EUR"),
	}); err != nil {
		t.Fatalf("second UpdateVariant() error = %v, want NULL rather than an empty-string collision", err)
	}
}

func TestArchiveVariantRemovesItFromCheckoutWithoutDeletingHistory(t *testing.T) {
	repository, db := newCatalogTestRepository(t)
	ctx := context.Background()
	variantID := seedVariant(t, db, "active", 1000)

	found, err := repository.FindActiveForCheckoutBatch(ctx, []uuid.UUID{variantID}, "en", "en")
	if err != nil || len(found) != 1 {
		t.Fatalf("variant not sellable before archiving: %d, %v", len(found), err)
	}

	archived, err := repository.ArchiveVariant(ctx, variantID)
	if err != nil || archived.Status != "archived" {
		t.Fatalf("ArchiveVariant() = (%+v, %v)", archived, err)
	}

	// Withdrawn from sale, but the row survives so stock, reservations,
	// returns and order links that reference it are not orphaned.
	found, err = repository.FindActiveForCheckoutBatch(ctx, []uuid.UUID{variantID}, "en", "en")
	if err != nil || len(found) != 0 {
		t.Fatalf("archived variant still sellable: %d, %v", len(found), err)
	}
	var remaining int64
	if err := db.Raw(`SELECT COUNT(*) FROM product_variants WHERE id = ?`, variantID).Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatal("archiving removed the row; history referencing this variant would be orphaned")
	}
}

func TestArchiveVariantIsIdempotent(t *testing.T) {
	repository, db := newCatalogTestRepository(t)
	ctx := context.Background()
	variantID := seedVariant(t, db, "active", 1000)

	if _, err := repository.ArchiveVariant(ctx, variantID); err != nil {
		t.Fatalf("first ArchiveVariant() error = %v", err)
	}
	// A retried admin request must not surface as a missing record.
	archived, err := repository.ArchiveVariant(ctx, variantID)
	if err != nil || archived.Status != "archived" {
		t.Fatalf("second ArchiveVariant() = (%+v, %v), want the archived variant", archived, err)
	}
}

func TestVariantMutationsReportAMissingVariant(t *testing.T) {
	repository, _ := newCatalogTestRepository(t)
	ctx := context.Background()
	missing := uuid.New()

	if _, err := repository.FindVariantForUpdate(ctx, missing); !errors.Is(err, domain.ErrProductNotFound) {
		t.Fatalf("FindVariantForUpdate() error = %v, want ErrProductNotFound", err)
	}
	if _, err := repository.UpdateVariant(ctx, missing, domain.UpdateVariantCommand{Status: "active", Price: mustTestMoney(t, 100, "EUR")}); !errors.Is(err, domain.ErrProductNotFound) {
		t.Fatalf("UpdateVariant() error = %v, want ErrProductNotFound", err)
	}
	if _, err := repository.ArchiveVariant(ctx, missing); !errors.Is(err, domain.ErrProductNotFound) {
		t.Fatalf("ArchiveVariant() error = %v, want ErrProductNotFound", err)
	}
}

func mustTestMoney(t *testing.T, amount int64, currency string) money.Money {
	t.Helper()
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// seedVariant creates a product with an English translation and one variant,
// which is the minimum FindActiveForCheckoutBatch needs to return a row.
func seedVariant(t *testing.T, db *gorm.DB, status string, price int64) uuid.UUID {
	t.Helper()
	productID, variantID := uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO products (id, status) VALUES (?, 'active')`, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_translations (product_id, locale, name, slug) VALUES (?, 'en', 'Cream', ?)`,
		productID, "cream-"+productID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_variants (id, product_id, sku, status, price_amount, currency, weight_grams)
		VALUES (?, ?, ?, ?, ?, 'EUR', 250)`, variantID, productID, "SKU-"+variantID.String(), status, price).Error; err != nil {
		t.Fatal(err)
	}
	return variantID
}

func newCatalogTestRepository(t *testing.T) (*VariantRepository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("catalog_test"),
		containerPostgres.WithUsername("catalog"),
		containerPostgres.WithPassword("catalog"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	if err := migrateCatalogDir(filepath.Join(root, "migrations", "core"), dsn, "schema_migrations"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO locales (code, name, is_active) VALUES ('en', 'English', true) ON CONFLICT DO NOTHING`).Error; err != nil {
		t.Fatal(err)
	}
	return NewVariantRepository(db), db
}

func migrateCatalogDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, catalogMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func catalogMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

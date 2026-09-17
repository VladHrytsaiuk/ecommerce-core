//go:build integration

package ordersnapshot

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

	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

// This adapter is the Returns module's only window onto an order, and it is one
// hand-written query. A join that silently returns nothing produces an empty
// contact and no mail at all — the exact silence this contact was added to end.

func TestTheSnapshotCarriesTheOrdersOwnContact(t *testing.T) {
	provider, db := newSnapshotProvider(t)
	orderID := seedOrder(t, db, "delivered")
	if err := db.Exec(`INSERT INTO order_contact_details (order_id, email, locale) VALUES (?, 'buyer@example.test', 'uk')`, orderID).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := provider.GetOrderSnapshot(context.Background(), orderID)
	if err != nil {
		t.Fatalf("GetOrderSnapshot() error = %v", err)
	}
	if snapshot.ContactEmail != "buyer@example.test" || snapshot.Locale != "uk" {
		t.Fatalf("contact = %q/%q, want the row stored against this order", snapshot.ContactEmail, snapshot.Locale)
	}
}

func TestAnOrderWithNoContactIsStillReadable(t *testing.T) {
	// A guest order placed before contact capture has no row. The join must
	// leave the fields empty rather than drop the order, or eligibility itself
	// would stop working for those buyers.
	provider, db := newSnapshotProvider(t)
	orderID := seedOrder(t, db, "delivered")

	snapshot, err := provider.GetOrderSnapshot(context.Background(), orderID)
	if err != nil {
		t.Fatalf("GetOrderSnapshot() error = %v", err)
	}
	if snapshot.OrderID != orderID {
		t.Fatalf("the order disappeared from an inner join: %+v", snapshot)
	}
	if snapshot.ContactEmail != "" || snapshot.Locale != "" {
		t.Fatalf("contact = %q/%q, want empty", snapshot.ContactEmail, snapshot.Locale)
	}
}

func TestOneOrdersContactNeverLeaksIntoAnother(t *testing.T) {
	provider, db := newSnapshotProvider(t)
	mine := seedOrder(t, db, "delivered")
	theirs := seedOrder(t, db, "delivered")
	if err := db.Exec(`INSERT INTO order_contact_details (order_id, email, locale) VALUES (?, 'stranger@example.test', 'en')`, theirs).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := provider.GetOrderSnapshot(context.Background(), mine)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ContactEmail != "" {
		t.Fatalf("snapshot of one order carries %q from another", snapshot.ContactEmail)
	}
}

func TestTheContactDoesNotDuplicateTheOrderRow(t *testing.T) {
	// order_contact_details is keyed by order_id, so the join is one-to-one.
	// If it ever stops being, the items below would be counted twice.
	provider, db := newSnapshotProvider(t)
	orderID := seedOrder(t, db, "delivered")
	if err := db.Exec(`INSERT INTO order_contact_details (order_id, email, locale) VALUES (?, 'buyer@example.test', 'uk')`, orderID).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := provider.GetOrderSnapshot(context.Background(), orderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Quantity != 2 {
		t.Fatalf("items = %+v, want the single seeded line", snapshot.Items)
	}
	if snapshot.Total.Amount() != 2500 {
		t.Fatalf("total = %d, want 2500", snapshot.Total.Amount())
	}
}

func TestAMissingOrderIsReportedNotFound(t *testing.T) {
	provider, _ := newSnapshotProvider(t)

	if _, err := provider.GetOrderSnapshot(context.Background(), uuid.New()); !errors.Is(err, returns.ErrReturnRequestNotFound) {
		t.Fatalf("GetOrderSnapshot() error = %v, want ErrReturnRequestNotFound", err)
	}
}

func seedOrder(t *testing.T, db *gorm.DB, status string) uuid.UUID {
	t.Helper()
	orderID := uuid.New()
	if err := db.Exec(`INSERT INTO orders (id, number, status, currency, subtotal_amount, tax_amount, shipping_amount, total_amount)
		VALUES (?, ?, ?, 'EUR', 2500, 0, 0, 2500)`, orderID, "ORDER-"+orderID.String()[:12], status).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO order_items (id, order_id, variant_id, product_name, quantity, unit_price_amount, total_amount, currency)
		VALUES (?, ?, NULL, 'Headphones', 2, 1250, 2500, 'EUR')`, uuid.New(), orderID).Error; err != nil {
		t.Fatal(err)
	}
	return orderID
}

func newSnapshotProvider(t *testing.T) (*Provider, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("snapshot_test"),
		containerPostgres.WithUsername("snapshot"),
		containerPostgres.WithPassword("snapshot"),
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
	for _, step := range []struct{ dir, table string }{
		{filepath.Join(root, "migrations", "core"), "schema_migrations"},
		{filepath.Join(root, "migrations", "modules", "orders"), "schema_migrations_module_orders"},
	} {
		if err := migrateSnapshotDir(step.dir, dsn, step.table); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewProvider(db), db
}

func migrateSnapshotDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, snapshotMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func snapshotMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

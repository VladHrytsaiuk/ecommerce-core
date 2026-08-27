//go:build integration

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/application"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	workflowPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/orderworkflow"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	promosApp "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
)

// TestReserveUsageLimitIsAtomic models concurrent checkout transaction hooks.
// With one available use, only one order can retain a reserved redemption.
func TestReserveUsageLimitIsAtomic(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine", containerPostgres.WithDatabase("promos_test"), containerPostgres.WithUsername("promos"), containerPostgres.WithPassword("promos"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	root := rootDir(t)
	if err := migrateDir(filepath.Join(root, "migrations", "core"), dsn, "schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if err := migrateDir(filepath.Join(root, "migrations", "modules", "promos"), dsn, "schema_migrations_module_promos"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	promoID := uuid.New()
	if err := db.Exec(`INSERT INTO promocodes (id, code, discount_type, discount_value, is_active, usage_limit) VALUES (?, 'ONE', 'percent', 1000, true, 1)`, promoID).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(db)
	const attempts = 8
	start := make(chan struct{})
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		orderID := uuid.New()
		if err := db.Exec(`INSERT INTO orders (id, number, status, currency, subtotal_amount, tax_amount, shipping_amount, total_amount) VALUES (?, ?, 'pending_payment', 'EUR', 1000, 0, 0, 1000)`, orderID, fmt.Sprintf("PROMO-%d", i)).Error; err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(orderID uuid.UUID) {
			defer wg.Done()
			<-start
			errs <- db.Transaction(func(tx *gorm.DB) error {
				return repository.Reserve(transaction.WithContext(ctx, tx), orderID, ordersDomain.Promotion{Code: "ONE", Type: "percent", Value: 1000})
			})
		}(orderID)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent promo reservations = %d, want 1", successes)
	}
	var redemptions, usage int
	if err := db.Raw(`SELECT COUNT(*) FROM promo_redemptions WHERE promo_id = ? AND status = 'reserved'`, promoID).Scan(&redemptions).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT usage_count FROM promocodes WHERE id = ?`, promoID).Scan(&usage).Error; err != nil {
		t.Fatal(err)
	}
	if redemptions != 1 || usage != 0 {
		t.Fatalf("redemptions/usage = %d/%d, want 1/0", redemptions, usage)
	}
}

func TestExpirePendingCheckoutReleasesInventoryAndPromo(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine", containerPostgres.WithDatabase("promos_expiry_test"), containerPostgres.WithUsername("promos"), containerPostgres.WithPassword("promos"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	root := rootDir(t)
	if err := migrateDir(filepath.Join(root, "migrations", "core"), dsn, "schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if err := migrateDir(filepath.Join(root, "migrations", "modules", "inventory"), dsn, "schema_migrations_module_inventory"); err != nil {
		t.Fatal(err)
	}
	if err := migrateDir(filepath.Join(root, "migrations", "modules", "promos"), dsn, "schema_migrations_module_promos"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	variantID, warehouseID, reservationID := seedExpiryReservation(t, db)
	promoID := uuid.New()
	if err := db.Exec(`INSERT INTO promocodes (id, code, discount_type, discount_value, is_active, usage_limit) VALUES (?, 'TTL10', 'percent', 1000, true, 1)`, promoID).Error; err != nil {
		t.Fatal(err)
	}
	promoRepository := NewRepository(db)
	workflow := workflowApp.NewService(workflowPostgres.NewRepository(db, false).WithTransactionHook(promosApp.NewWorkflowHook(promoRepository)))
	price := mustMoney(1000, "EUR")
	discounted := mustMoney(900, "EUR")
	discount := mustMoney(100, "EUR")
	order, err := workflow.CreatePendingCheckout(ctx, ordersDomain.Draft{Number: "TTL-1", ExpiresAt: time.Now().Add(time.Minute), Subtotal: discounted, Tax: mustMoney(0, "EUR"), Shipping: mustMoney(0, "EUR"), Total: discounted, PaymentProvider: "fake", Items: []ordersDomain.Item{{VariantID: &variantID, ProductName: "Cream", SKU: "TTL-CREAM", Quantity: 1, UnitPrice: price, Total: price}}, Promotion: &ordersDomain.Promotion{Code: "TTL10", Type: "percent", Value: 1000, Discount: discount}}, []uuid.UUID{reservationID}, workflowDomain.CheckoutAttemptRequest{Provider: "fake", IdempotencyKey: "ttl-checkout", Amount: discounted, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE orders SET expires_at = CURRENT_TIMESTAMP - INTERVAL '1 second' WHERE id = ?`, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := workflow.ExpirePendingCheckout(ctx, time.Now().UTC())
	if err != nil || !expired {
		t.Fatalf("ExpirePendingCheckout() = (%t, %v)", expired, err)
	}
	var orderStatus, reservationStatus, redemptionStatus, attemptStatus string
	var onHand, reserved int
	if err := db.Raw(`SELECT status FROM orders WHERE id = ?`, order.ID).Scan(&orderStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status FROM inventory_reservations WHERE id = ?`, reservationID).Scan(&reservationStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status FROM promo_redemptions WHERE order_id = ?`, order.ID).Scan(&redemptionStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status FROM payment_checkout_attempts WHERE order_id = ?`, order.ID).Scan(&attemptStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT quantity_on_hand, quantity_reserved FROM stock_items WHERE variant_id = ? AND warehouse_id = ?`, variantID, warehouseID).Row().Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if orderStatus != "cancelled" || reservationStatus != "released" || redemptionStatus != "released" || attemptStatus != "failed" || onHand != 5 || reserved != 0 {
		t.Fatalf("expiry state order=%s reservation=%s promo=%s attempt=%s stock=%d/%d", orderStatus, reservationStatus, redemptionStatus, attemptStatus, onHand, reserved)
	}
}

func seedExpiryReservation(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	variantID, warehouseID, reservationID, productID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO locales (code, name, is_default) VALUES ('en', 'English', true)`, nil},
		{`INSERT INTO products (id, status) VALUES (?, 'active')`, []any{productID}},
		{`INSERT INTO product_variants (id, product_id, sku, status, price_amount, currency) VALUES (?, ?, 'TTL-CREAM', 'active', 1000, 'EUR')`, []any{variantID, productID}},
		{`INSERT INTO warehouses (id, code, name) VALUES (?, 'ttl', 'TTL')`, []any{warehouseID}},
		{`INSERT INTO stock_items (id, variant_id, warehouse_id, quantity_on_hand, quantity_reserved) VALUES (?, ?, ?, 5, 1)`, []any{uuid.New(), variantID, warehouseID}},
		{`INSERT INTO inventory_reservations (id, idempotency_key, variant_id, warehouse_id, quantity, expires_at) VALUES (?, ?, ?, ?, 1, ?)`, []any{reservationID, uuid.New(), variantID, warehouseID, time.Now().Add(time.Hour)}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return variantID, warehouseID, reservationID
}

func migrateDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, migrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	err = m.Up()
	if err == migrate.ErrNoChange {
		return nil
	}
	return err
}
func migrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
func rootDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func mustMoney(amount int64, currency string) money.Money {
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return value
}

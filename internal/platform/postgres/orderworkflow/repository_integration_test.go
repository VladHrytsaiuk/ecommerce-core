//go:build integration

package orderworkflow

import (
	"context"
	"net/url"
	"path/filepath"
	"runtime"
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
)

func TestWorkflowPersistsOrderAndCommitsReservationExactlyOnce(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx,
		"postgres:16-alpine",
		containerPostgres.WithDatabase("workflow_test"),
		containerPostgres.WithUsername("workflow"),
		containerPostgres.WithPassword("workflow"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get PostgreSQL connection string: %v", err)
	}
	root := repositoryRoot(t)
	if err := applyMigrations(filepath.Join(root, "migrations", "core"), databaseURL, "schema_migrations"); err != nil {
		t.Fatalf("migrate core: %v", err)
	}
	if err := applyMigrations(filepath.Join(root, "migrations", "modules", "inventory"), databaseURL, "schema_migrations_module_inventory"); err != nil {
		t.Fatalf("migrate inventory: %v", err)
	}

	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	variantID, _, reservationID := seedReservation(t, db)
	price, _ := money.New(1000, "EUR")
	service := workflowApp.NewService(NewRepository(db))
	order, err := service.CreatePending(ctx, ordersDomain.Draft{
		Number: "ES-300", Subtotal: price, Tax: money.Money{Currency: "EUR"}, Total: price, PaymentProvider: "fake", DeliveryProvider: "novaposhta",
		Delivery: &ordersDomain.DeliveryDetails{RecipientName: "Iryna Customer", RecipientPhone: "+34123456789", CountryCode: "ES", City: "Madrid", LocalityID: "madrid", ServicePointID: "branch-1"},
		Items:    []ordersDomain.Item{{VariantID: &variantID, ProductName: "Cream", SKU: "CREAM-50", Quantity: 1, UnitPrice: price, Total: price, UnitWeightGrams: 275}},
	}, []uuid.UUID{reservationID})
	if err != nil {
		t.Fatalf("CreatePending() error = %v", err)
	}
	confirmation := workflowDomain.PaymentConfirmation{PaymentAttempt: workflowDomain.PaymentAttempt{OrderID: order.ID, Provider: "fake", ProviderReference: "payment-300", Amount: price}, Status: "paid"}
	if err := service.RegisterPayment(ctx, confirmation.PaymentAttempt); err != nil {
		t.Fatalf("RegisterPayment() error = %v", err)
	}
	assertOrderAndReservation(t, db, order.ID, reservationID, "pending_payment", "active", 5, 1)

	if err := service.MarkPaid(ctx, confirmation); err != nil {
		t.Fatalf("MarkPaid() error = %v", err)
	}
	if err := service.MarkPaid(ctx, confirmation); err != nil {
		t.Fatalf("idempotent MarkPaid() error = %v", err)
	}
	assertOrderAndReservation(t, db, order.ID, reservationID, "paid", "committed", 4, 0)
	var jobs int
	var recipient string
	var unitWeightGrams int
	var paymentStatus, paymentReference string
	if err := db.Raw(`SELECT COUNT(*) FROM delivery_jobs WHERE order_id = ? AND provider = 'novaposhta' AND status = 'pending'`, order.ID).Scan(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT recipient_name FROM order_delivery_details WHERE order_id = ?`, order.ID).Scan(&recipient).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT unit_weight_grams FROM order_items WHERE order_id = ?`, order.ID).Scan(&unitWeightGrams).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status, provider_reference FROM payments WHERE order_id = ?`, order.ID).Row().Scan(&paymentStatus, &paymentReference); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || recipient != "Iryna Customer" || unitWeightGrams != 275 || paymentStatus != "paid" || paymentReference != "payment-300" {
		t.Fatalf("delivery snapshot/jobs/weight/payment = %q/%d/%d/%s/%s", recipient, jobs, unitWeightGrams, paymentStatus, paymentReference)
	}
}

func seedReservation(t *testing.T, db *gorm.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	variantID, warehouseID, reservationID := uuid.New(), uuid.New(), uuid.New()
	productID := uuid.New()
	if err := db.Exec(`INSERT INTO locales (code, name, is_default) VALUES ('en', 'English', true)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO products (id, status) VALUES (?, 'active')`, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO product_variants (id, product_id, sku, status, price_amount, currency) VALUES (?, ?, 'CREAM-50', 'active', 1000, 'EUR')`, variantID, productID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO warehouses (id, code, name) VALUES (?, 'main', 'Main')`, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO stock_items (id, variant_id, warehouse_id, quantity_on_hand, quantity_reserved) VALUES (?, ?, ?, 5, 1)`, uuid.New(), variantID, warehouseID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO inventory_reservations (id, idempotency_key, variant_id, warehouse_id, quantity, expires_at) VALUES (?, ?, ?, ?, 1, ?)`, reservationID, uuid.New(), variantID, warehouseID, time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	return variantID, warehouseID, reservationID
}

func assertOrderAndReservation(t *testing.T, db *gorm.DB, orderID, reservationID uuid.UUID, wantOrderStatus, wantReservationStatus string, wantOnHand, wantReserved int) {
	t.Helper()
	var orderStatus, reservationStatus string
	var reservationOrderID *uuid.UUID
	var onHand, reserved int
	if err := db.Raw(`SELECT status FROM orders WHERE id = ?`, orderID).Scan(&orderStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status, order_id FROM inventory_reservations WHERE id = ?`, reservationID).Row().Scan(&reservationStatus, &reservationOrderID); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT quantity_on_hand, quantity_reserved FROM stock_items LIMIT 1`).Row().Scan(&onHand, &reserved); err != nil {
		t.Fatal(err)
	}
	if orderStatus != wantOrderStatus || reservationStatus != wantReservationStatus || reservationOrderID == nil || *reservationOrderID != orderID || onHand != wantOnHand || reserved != wantReserved {
		t.Fatalf("state = order:%s reservation:%s reservationOrder:%v stock:%d/%d", orderStatus, reservationStatus, reservationOrderID, onHand, reserved)
	}
}

func applyMigrations(dir, databaseURL, table string) error {
	m, err := migrate.New("file://"+dir, migrationURL(databaseURL, table))
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func migrationURL(databaseURL, table string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

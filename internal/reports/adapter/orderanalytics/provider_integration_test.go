//go:build integration

package orderanalytics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	reports "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
)

// TestRangeReadAgreesWithThePerOrderRead is the safety net for replacing the
// per-order loop with batched queries. The two paths must describe an order
// identically; anything else is a silent change to what a rebuilt report says.
func TestRangeReadAgreesWithThePerOrderRead(t *testing.T) {
	provider, db, _ := newAnalyticsProvider(t)
	ctx := context.Background()
	paidOnly := seedPaidOrder(t, db, 2, 1500)
	refunded := seedPaidOrder(t, db, 1, 900)
	seedRefundEvent(t, db, refunded)

	snapshots, err := provider.GetSnapshotsByDateRange(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetSnapshotsByDateRange() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snapshots))
	}
	byOrder := map[uuid.UUID]int{}
	for index, snapshot := range snapshots {
		byOrder[snapshot.OrderID] = index
	}

	for _, orderID := range []uuid.UUID{paidOnly, refunded} {
		single, err := provider.GetAnalyticsSnapshot(ctx, orderID)
		if err != nil {
			t.Fatalf("GetAnalyticsSnapshot(%s) error = %v", orderID, err)
		}
		index, present := byOrder[orderID]
		if !present {
			t.Fatalf("order %s missing from the range read", orderID)
		}
		ranged := snapshots[index]
		// The range read adds the refund facts, which the per-order read has
		// never carried; everything else must match exactly.
		ranged.RefundedAt, ranged.RefundedEventID = nil, nil
		if single.OrderID != ranged.OrderID || single.TotalMinor != ranged.TotalMinor ||
			single.SubtotalMinor != ranged.SubtotalMinor || single.TaxMinor != ranged.TaxMinor ||
			single.ShippingMinor != ranged.ShippingMinor || single.DiscountMinor != ranged.DiscountMinor ||
			single.Currency != ranged.Currency || single.Channel != ranged.Channel ||
			*single.PaidEventID != *ranged.PaidEventID || !single.PaidAt.Equal(*ranged.PaidAt) {
			t.Fatalf("order %s differs between the two reads:\n  single = %+v\n  ranged = %+v", orderID, single, ranged)
		}
		if len(single.PurchasedProductItems) != len(ranged.PurchasedProductItems) {
			t.Fatalf("order %s items = %d single vs %d ranged", orderID, len(single.PurchasedProductItems), len(ranged.PurchasedProductItems))
		}
		for i := range single.PurchasedProductItems {
			// The item carries pointers, so == would compare addresses from
			// two separate reads and never match.
			if describeItem(single.PurchasedProductItems[i]) != describeItem(ranged.PurchasedProductItems[i]) {
				t.Fatalf("order %s item %d differs:\n  single = %s\n  ranged = %s",
					orderID, i, describeItem(single.PurchasedProductItems[i]), describeItem(ranged.PurchasedProductItems[i]))
			}
		}
	}

	// The refund facts belong only to the order that has a refund event.
	if snapshots[byOrder[refunded]].RefundedAt == nil || snapshots[byOrder[refunded]].RefundedEventID == nil {
		t.Fatal("the refunded order carries no refund facts")
	}
	if snapshots[byOrder[paidOnly]].RefundedAt != nil || snapshots[byOrder[paidOnly]].RefundedEventID != nil {
		t.Fatal("an order with no refund event was given refund facts")
	}
}

// TestRangeReadCostDoesNotGrowWithTheNumberOfOrders is the finding itself. The
// previous implementation issued three queries per order inside the single
// transaction that also holds the projection lock, so a month of five thousand
// orders was fifteen thousand round trips in one transaction.
func TestRangeReadCostDoesNotGrowWithTheNumberOfOrders(t *testing.T) {
	provider, db, queries := newAnalyticsProvider(t)
	ctx := context.Background()
	for i := 0; i < 12; i++ {
		seedRefundEvent(t, db, seedPaidOrder(t, db, 2, 1000))
	}

	queries.Store(0)
	snapshots, err := provider.GetSnapshotsByDateRange(ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetSnapshotsByDateRange() error = %v", err)
	}
	if len(snapshots) != 12 {
		t.Fatalf("snapshots = %d, want 12", len(snapshots))
	}
	if count := queries.Load(); count > 4 {
		t.Fatalf("%d queries for 12 orders; the read must not scale with the order count", count)
	}
}

func TestRangeReadIgnoresOrdersOutsideTheWindow(t *testing.T) {
	provider, db, _ := newAnalyticsProvider(t)
	inWindow := seedPaidOrder(t, db, 1, 500)
	outOfWindow := seedPaidOrder(t, db, 1, 500)
	if err := db.Exec(`UPDATE domain_events SET occurred_at = CURRENT_TIMESTAMP - INTERVAL '10 days' WHERE aggregate_id = ?`, outOfWindow).Error; err != nil {
		t.Fatal(err)
	}

	snapshots, err := provider.GetSnapshotsByDateRange(context.Background(), time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetSnapshotsByDateRange() error = %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].OrderID != inWindow {
		t.Fatalf("snapshots = %d, want only the in-window order %s", len(snapshots), inWindow)
	}
}

func TestRangeReadReturnsNothingForAnEmptyWindow(t *testing.T) {
	provider, db, _ := newAnalyticsProvider(t)
	seedPaidOrder(t, db, 1, 500)

	snapshots, err := provider.GetSnapshotsByDateRange(context.Background(), time.Now().Add(-72*time.Hour), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("GetSnapshotsByDateRange() error = %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("snapshots = %d, want none", len(snapshots))
	}
}

func seedPaidOrder(t *testing.T, db *gorm.DB, itemCount int, unitMinor int64) uuid.UUID {
	t.Helper()
	orderID, cartID := uuid.New(), uuid.New()
	total := unitMinor * int64(itemCount)
	if err := db.Exec(`INSERT INTO carts (id, session_id, status) VALUES (?, ?, 'active')`, cartID, uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO orders (id, number, cart_id, status, currency, subtotal_amount, tax_amount, shipping_amount, total_amount)
		VALUES (?, ?, ?, 'paid', 'EUR', ?, 0, 0, ?)`,
		orderID, "ORD-"+orderID.String()[:8], cartID, total, total).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < itemCount; i++ {
		productID, variantID := uuid.New(), uuid.New()
		if err := db.Exec(`INSERT INTO products (id, status) VALUES (?, 'active')`, productID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO product_variants (id, product_id, sku, status, price_amount, currency)
			VALUES (?, ?, ?, 'active', ?, 'EUR')`, variantID, productID, "SKU-"+variantID.String(), unitMinor).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO order_items (id, order_id, variant_id, product_name, quantity, unit_price_amount, total_amount, currency)
			VALUES (?, ?, ?, 'Cream', 1, ?, ?, 'EUR')`, uuid.New(), orderID, variantID, unitMinor, unitMinor).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedEvent(t, db, orderID, events.TopicOrderPaid)
	return orderID
}

func seedRefundEvent(t *testing.T, db *gorm.DB, orderID uuid.UUID) {
	t.Helper()
	seedEvent(t, db, orderID, events.TopicOrderRefunded)
}

func seedEvent(t *testing.T, db *gorm.DB, orderID uuid.UUID, topic string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO domain_events (id, topic, aggregate_type, aggregate_id, idempotency_key, payload, occurred_at)
		VALUES (?, ?, 'order', ?, ?, ?, CURRENT_TIMESTAMP)`,
		uuid.New(), topic, orderID, uuid.New(), `{"version":1,"order_id":"`+orderID.String()+`"}`).Error; err != nil {
		t.Fatal(err)
	}
}

func describeItem(item reports.PurchasedProductItem) string {
	product, variant := "nil", "nil"
	if item.ProductID != nil {
		product = item.ProductID.String()
	}
	if item.VariantID != nil {
		variant = item.VariantID.String()
	}
	return fmt.Sprintf("product=%s variant=%s qty=%d unit=%d total=%d %s",
		product, variant, item.Quantity, item.UnitMinor, item.TotalMinor, item.Currency)
}

// sharedDSN is started once for the package. Four containers in sequence
// proved flaky on a loaded machine, and nothing here needs its own server —
// each test truncates first.
var sharedDSN string

func TestMain(m *testing.M) {
	if !startSharedPostgres() {
		os.Exit(m.Run())
	}
	os.Exit(m.Run())
}

func startSharedPostgres() bool {
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("reports_test"),
		postgresContainer.WithUsername("reports"),
		postgresContainer.WithPassword("reports"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		return false
	}
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return false
	}
	sharedDSN = dsn
	return applyCoreSchema(dsn) == nil
}

// applyCoreSchema runs the core migrations once, since they are not idempotent
// and every test shares this server.
func applyCoreSchema(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		return err
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations", "core")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			return readErr
		}
		if execErr := db.Exec(string(raw)).Error; execErr != nil {
			return fmt.Errorf("apply %s: %w", name, execErr)
		}
	}
	return nil
}

// newAnalyticsProvider returns the provider, a raw handle for seeding, and a
// counter of the queries the provider issues.
func newAnalyticsProvider(t *testing.T) (*Provider, *gorm.DB, *atomic.Int64) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	if sharedDSN == "" {
		t.Skip("no PostgreSQL container available")
	}
	dsn := sharedDSN
	ctx := context.Background()
	_ = ctx
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`TRUNCATE domain_events, order_items, orders, product_variants, products, carts RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatal(err)
	}

	// A separate handle whose logger counts statements, so the provider's cost
	// can be measured without counting the seeding above. A logger rather than
	// a callback: Raw().Scan() does not run the raw-callback chain, so
	// registering there counted nothing and the assertion passed vacuously.
	var queries atomic.Int64
	counted, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: &countingLogger{queries: &queries}})
	if err != nil {
		t.Fatal(err)
	}
	return NewProvider(counted), db, &queries
}

type countingLogger struct{ queries *atomic.Int64 }

func (l *countingLogger) LogMode(gormLogger.LogLevel) gormLogger.Interface { return l }
func (*countingLogger) Info(context.Context, string, ...interface{})       {}
func (*countingLogger) Warn(context.Context, string, ...interface{})       {}
func (*countingLogger) Error(context.Context, string, ...interface{})      {}
func (l *countingLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	l.queries.Add(1)
}

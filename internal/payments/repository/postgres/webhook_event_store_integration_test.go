//go:build integration

package postgres

import (
	"context"
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
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

// TestClaimIsExclusiveAcrossConcurrentReplicas models the same provider retry
// reaching two API replicas at once. Reading processing_status without a lock
// let both observe 'processing' and run the callback concurrently; the loser
// then reported a payment failure that had not happened.
func TestClaimIsExclusiveAcrossConcurrentReplicas(t *testing.T) {
	db, orderID := newWebhookTestDB(t)
	store := NewWebhookEventStore(db)
	event := newWebhookEvent(t, orderID, "evt-concurrent")

	const replicas = 8
	claims := make([]bool, replicas)
	errs := make([]error, replicas)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for replica := range replicas {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claims[replica], errs[replica] = store.Claim(context.Background(), "stripe", event)
		}()
	}
	close(start)
	wg.Wait()

	granted := 0
	for replica := range replicas {
		if errs[replica] != nil {
			t.Fatalf("replica %d: Claim() error = %v", replica, errs[replica])
		}
		if claims[replica] {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("Claim() granted %d replicas, want exactly 1", granted)
	}
}

// TestClaimTakesOverAnExpiredLease preserves the crash-recovery behaviour the
// unlocked status read was there to provide: a replica that died mid-callback
// must not strand the event forever.
func TestClaimTakesOverAnExpiredLease(t *testing.T) {
	db, orderID := newWebhookTestDB(t)
	store := NewWebhookEventStore(db)
	event := newWebhookEvent(t, orderID, "evt-crashed")

	claimed, err := store.Claim(context.Background(), "stripe", event)
	if err != nil || !claimed {
		t.Fatalf("first Claim() = (%t, %v), want granted", claimed, err)
	}
	if again, err := store.Claim(context.Background(), "stripe", event); err != nil || again {
		t.Fatalf("Claim() during a live lease = (%t, %v), want refused", again, err)
	}

	// Simulate the holder dying: age the lease past its window.
	if err := db.Exec(`UPDATE payment_webhook_events SET locked_at = CURRENT_TIMESTAMP - INTERVAL '10 minutes' WHERE event_id = ?`, event.EventID).Error; err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Claim(context.Background(), "stripe", event)
	if err != nil || !recovered {
		t.Fatalf("Claim() after the lease expired = (%t, %v), want taken over", recovered, err)
	}
}

// TestClaimNeverReprocessesACompletedEvent guards the idempotency contract the
// whole table exists for: providers retry successful callbacks routinely.
func TestClaimNeverReprocessesACompletedEvent(t *testing.T) {
	db, orderID := newWebhookTestDB(t)
	store := NewWebhookEventStore(db)
	event := newWebhookEvent(t, orderID, "evt-done")
	ctx := context.Background()

	if claimed, err := store.Claim(ctx, "stripe", event); err != nil || !claimed {
		t.Fatalf("Claim() = (%t, %v), want granted", claimed, err)
	}
	if err := store.MarkProcessed(ctx, "stripe", event.EventID); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	// Even well past the lease window a processed event stays a no-op.
	if err := db.Exec(`UPDATE payment_webhook_events SET locked_at = CURRENT_TIMESTAMP - INTERVAL '10 minutes' WHERE event_id = ?`, event.EventID).Error; err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.Claim(ctx, "stripe", event); err != nil || claimed {
		t.Fatalf("Claim() on a processed event = (%t, %v), want refused", claimed, err)
	}
}

// TestAbandonReleasesTheClaimForRetry covers the path the webhook service uses
// when applying the event failed: the provider will retry and must get through.
func TestAbandonReleasesTheClaimForRetry(t *testing.T) {
	db, orderID := newWebhookTestDB(t)
	store := NewWebhookEventStore(db)
	event := newWebhookEvent(t, orderID, "evt-abandoned")
	ctx := context.Background()

	if claimed, err := store.Claim(ctx, "stripe", event); err != nil || !claimed {
		t.Fatalf("Claim() = (%t, %v), want granted", claimed, err)
	}
	if err := store.Abandon(ctx, "stripe", event.EventID); err != nil {
		t.Fatalf("Abandon() error = %v", err)
	}
	if claimed, err := store.Claim(ctx, "stripe", event); err != nil || !claimed {
		t.Fatalf("Claim() after Abandon() = (%t, %v), want granted", claimed, err)
	}
}

func newWebhookEvent(t *testing.T, orderID uuid.UUID, eventID string) paymentsDomain.PaymentEvent {
	t.Helper()
	amount, err := money.NewMoney(1000, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	return paymentsDomain.PaymentEvent{
		EventID: eventID, Provider: "stripe", OrderID: orderID,
		ProviderReference: "pi_" + eventID, Status: "paid", Amount: amount,
		OccurredAt: time.Now().UTC(),
	}
}

// newWebhookTestDB provisions core schema and one order, since
// payment_webhook_events carries a foreign key to orders.
func newWebhookTestDB(t *testing.T) (*gorm.DB, uuid.UUID) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("payments_test"),
		containerPostgres.WithUsername("payments"),
		containerPostgres.WithPassword("payments"),
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
	if err := migrateWebhookDir(filepath.Join(root, "migrations", "core"), dsn, "schema_migrations"); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	orderID := uuid.New()
	if err := db.Exec(`INSERT INTO orders (id, number, status, currency, subtotal_amount, tax_amount, shipping_amount, total_amount, payment_provider, delivery_provider, expires_at)
		VALUES (?, 'WH-1', 'pending_payment', 'EUR', 1000, 0, 0, 1000, 'stripe', '', CURRENT_TIMESTAMP + INTERVAL '1 hour')`, orderID).Error; err != nil {
		t.Fatal(err)
	}
	return db, orderID
}

func migrateWebhookDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, webhookMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func webhookMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

// TestAReclaimedJobRefusesTheOldWorkersCompletion is the duplicate-waybill
// scenario. The first worker is still at the carrier when its lease expires;
// the reclaim sweep returns the job and a second worker claims it. The first
// worker must not be able to write its result over the newer claim's.
func TestAReclaimedJobRefusesTheOldWorkersCompletion(t *testing.T) {
	store, db := newDeliveryTestStore(t)
	ctx := context.Background()
	seedDeliveryJob(t, db)

	first, err := store.Claim(ctx, time.Now().UTC())
	if err != nil || first == nil {
		t.Fatalf("first Claim() = (%v, %v)", first, err)
	}

	// Age the lease past the reclaim window, then let a second worker take it.
	expireLease(t, db, first.ID)
	second, err := store.Claim(ctx, time.Now().UTC())
	if err != nil || second == nil {
		t.Fatalf("second Claim() = (%v, %v)", second, err)
	}
	if second.LockToken == first.LockToken {
		t.Fatal("the reclaim reused the previous token; the old worker would still be authorized")
	}

	if err := store.Complete(ctx, *first, domain.ShipmentResult{ProviderReference: "stale", TrackingNumber: "STALE"}); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("stale Complete() error = %v, want ErrLeaseLost", err)
	}
	if err := store.Complete(ctx, *second, domain.ShipmentResult{ProviderReference: "live", TrackingNumber: "LIVE"}); err != nil {
		t.Fatalf("live Complete() error = %v", err)
	}

	var tracking string
	if err := db.Raw(`SELECT tracking_number FROM deliveries WHERE order_id = ?`, second.OrderID).Row().Scan(&tracking); err != nil {
		t.Fatal(err)
	}
	if tracking != "LIVE" {
		t.Fatalf("tracking_number = %q, want the newer claim's shipment", tracking)
	}
}

// TestAStaleWorkerCannotFailALiveClaim covers the other direction: the old
// worker's error must not bury a job the newer claim is still working on.
func TestAStaleWorkerCannotFailALiveClaim(t *testing.T) {
	store, db := newDeliveryTestStore(t)
	ctx := context.Background()
	seedDeliveryJob(t, db)

	first, err := store.Claim(ctx, time.Now().UTC())
	if err != nil || first == nil {
		t.Fatalf("first Claim() = (%v, %v)", first, err)
	}
	expireLease(t, db, first.ID)
	if _, err := store.Claim(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	for name, write := range map[string]func() error{
		"dead":  func() error { return store.Dead(ctx, *first, errors.New("stale")) },
		"retry": func() error { return store.Retry(ctx, *first, errors.New("stale"), time.Now().UTC(), false) },
		"fail":  func() error { return store.Fail(ctx, *first, errors.New("stale")) },
	} {
		if err := write(); !errors.Is(err, domain.ErrLeaseLost) {
			t.Fatalf("stale %s error = %v, want ErrLeaseLost", name, err)
		}
	}

	var status string
	if err := db.Raw(`SELECT status FROM delivery_jobs WHERE id = ?`, first.ID).Row().Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("status = %q, want the live claim still processing", status)
	}
}

// TestARetryRecordsWhetherTheFailureCreatedNothing checks the flag survives
// the round trip, since the next claim's decision to re-dispatch rests on it.
func TestARetryRecordsWhetherTheFailureCreatedNothing(t *testing.T) {
	store, db := newDeliveryTestStore(t)
	ctx := context.Background()
	seedDeliveryJob(t, db)

	claimed, err := store.Claim(ctx, time.Now().UTC())
	if err != nil || claimed == nil {
		t.Fatalf("Claim() = (%v, %v)", claimed, err)
	}
	if claimed.LastFailureWasDefinite {
		t.Fatal("a job that has never failed reports a definite failure")
	}
	if err := store.Retry(ctx, *claimed, errors.New("rejected"), time.Now().UTC().Add(-time.Minute), true); err != nil {
		t.Fatal(err)
	}

	reclaimed, err := store.Claim(ctx, time.Now().UTC())
	if err != nil || reclaimed == nil {
		t.Fatalf("re-Claim() = (%v, %v)", reclaimed, err)
	}
	if !reclaimed.LastFailureWasDefinite {
		t.Fatal("the recorded failure was not carried into the next claim; a safe retry would be parked")
	}
}

func expireLease(t *testing.T, db *gorm.DB, jobID uuid.UUID) {
	t.Helper()
	if err := db.Exec(`UPDATE delivery_jobs SET locked_at = CURRENT_TIMESTAMP - INTERVAL '10 minutes' WHERE id = ?`, jobID).Error; err != nil {
		t.Fatal(err)
	}
}

func seedDeliveryJob(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	orderID, jobID := uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO orders (id, number, status, currency, subtotal_amount, total_amount) VALUES (?, ?, 'paid', 'EUR', 1000, 1000)`, []any{orderID, "ORD-" + orderID.String()[:8]}},
		{`INSERT INTO order_delivery_details (order_id, recipient_name, recipient_phone, country_code, city, line1) VALUES (?, 'Buyer', '+380000000000', 'UA', 'Kyiv', 'Street 1')`, []any{orderID}},
		// available_at is set explicitly rather than defaulted: the container's
		// clock and this process's differ by enough that a CURRENT_TIMESTAMP
		// default can sit just after the `now` the claim is given.
		{`INSERT INTO delivery_jobs (id, order_id, provider, idempotency_key, status, available_at) VALUES (?, ?, 'fake', ?, 'pending', CURRENT_TIMESTAMP - INTERVAL '1 minute')`, []any{jobID, orderID, uuid.New()}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatalf("%s: %v", statement.sql, err)
		}
	}
	return jobID
}

func newDeliveryTestStore(t *testing.T) (*JobStore, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("delivery_test"),
		postgresContainer.WithUsername("delivery"),
		postgresContainer.WithPassword("delivery"),
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
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		t.Fatal(err)
	}
	// delivery_jobs lives in core and its fencing arrived in a later core
	// migration, so the whole core directory is applied in order.
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "migrations", "core")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(raw)).Error; err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return NewJobStore(db), db
}

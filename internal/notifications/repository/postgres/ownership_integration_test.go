//go:build integration

package postgres

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// TestClaimDueNeverTakesAnOrderPaidReceipt is the defect this table's
// ownership column exists to prevent. The scheduler's claim had no filter, so
// it took the outbox handler's receipts, found their payload column NULL —
// their render data is encrypted elsewhere — and recorded them dead. The
// handler then treated 'dead' as settled and acknowledged the outbox entry, so
// the order confirmation was never sent and nothing was left to retry.
func TestClaimDueNeverTakesAnOrderPaidReceipt(t *testing.T) {
	repository, db := newNotificationTestRepository(t)
	ctx := context.Background()
	receiptID := seedOrderPaidJob(t, db)

	claimed, err := repository.ClaimDue(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if claimed != nil {
		t.Fatalf("ClaimDue() claimed an order-paid receipt (%s); it belongs to the outbox handler", claimed.ID)
	}

	// The receipt must be untouched and still deliverable, not moved into a
	// state the handler reads as already settled.
	var status string
	var retryCount int
	if err := db.Raw(`SELECT status, retry_count FROM notification_jobs WHERE id = ?`, receiptID).Row().Scan(&status, &retryCount); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || retryCount != 0 {
		t.Fatalf("receipt = (%s, %d attempts), want it left pending and unattempted", status, retryCount)
	}
}

// TestClaimDueStillTakesItsOwnJobs guards the other direction: the filter must
// not stop the scheduler doing its own work.
// TestAScheduledJobCarriesItsLocaleToTheWorker covers the other half of the
// same row. The locale column was written as a hard-coded "en" and then never
// read back: the worker rendered every scheduled email in English, so a store
// that had seeded both English and its own templates sent English to everyone.
func TestAScheduledJobCarriesItsLocaleToTheWorker(t *testing.T) {
	repository, db := newNotificationTestRepository(t)
	seedScheduledJob(t, db, "back_in_stock")

	claimed, err := repository.ClaimDue(context.Background(), time.Now().UTC())
	if err != nil || claimed == nil {
		t.Fatalf("ClaimDue() = (%v, %v)", claimed, err)
	}
	if claimed.Locale != "uk" {
		t.Fatalf("claimed locale = %q, want the store's configured locale", claimed.Locale)
	}
}

func TestClaimDueStillTakesItsOwnJobs(t *testing.T) {
	repository, db := newNotificationTestRepository(t)
	ctx := context.Background()
	seedOrderPaidJob(t, db)
	scheduledID := seedScheduledJob(t, db, "back_in_stock")

	claimed, err := repository.ClaimDue(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if claimed == nil || claimed.ID != scheduledID {
		t.Fatalf("ClaimDue() = %v, want the scheduled job %s", claimed, scheduledID)
	}
	if len(claimed.Payload) == 0 {
		t.Fatal("claimed job carries no payload to render from")
	}
}

// TestClaimOrderPaidJobRefusesAScheduledJob covers the mirror image. The
// handler claims by id, so it cannot scan into the scheduler's rows by
// accident — but an id can be wrong, and this is the row it must refuse.
func TestClaimOrderPaidJobRefusesAScheduledJob(t *testing.T) {
	repository, db := newNotificationTestRepository(t)
	scheduledID := seedScheduledJob(t, db, "back_in_stock")

	claimed, ok, err := repository.ClaimOrderPaidJob(context.Background(), scheduledID, time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatalf("ClaimOrderPaidJob() error = %v", err)
	}
	if ok || claimed != nil {
		t.Fatal("ClaimOrderPaidJob() claimed a scheduled job that belongs to the durable worker")
	}
}

// TestTheDatabaseRefusesAJobWithTheWrongPayloadShape checks the constraint
// rather than the query. A future writer that forgets the ownership rule has
// to fail at the insert, not in whichever worker picks the row up.
func TestTheDatabaseRefusesAJobWithTheWrongPayloadShape(t *testing.T) {
	_, db := newNotificationTestRepository(t)

	// A scheduler job with nothing to render from.
	err := db.Exec(`INSERT INTO notification_jobs (id, event_id, dedupe_key, channel, recipient_email, locale, template_key, payload_ciphertext, status, provider, dispatcher, type)
		VALUES (?, ?, ?, 'email', 'buyer@example.com', 'en', 'back_in_stock', '{}', 'pending', 'scheduled', 'scheduler', 'back_in_stock')`,
		uuid.New(), uuid.New(), uuid.NewString()).Error
	if err == nil {
		t.Fatal("a scheduler job with no payload was accepted; the worker would record it dead on the first claim")
	}

	// An outbox receipt carrying plaintext, which is what the encryption of
	// payload_ciphertext exists to prevent.
	err = db.Exec(`INSERT INTO notification_jobs (id, event_id, order_id, dedupe_key, channel, recipient_email, locale, template_key, payload_ciphertext, status, provider, dispatcher, payload)
		VALUES (?, ?, ?, ?, 'email', 'buyer@example.com', 'en', 'order_paid', 'cipher', 'pending', 'smtp', 'outbox', '{"email":"buyer@example.com"}')`,
		uuid.New(), uuid.New(), uuid.New(), uuid.NewString()).Error
	if err == nil {
		t.Fatal("an outbox receipt with a plaintext payload was accepted")
	}
}

// seedOrderPaidJob writes a receipt the way CreateOrderPaidJob does: render
// data encrypted into payload_ciphertext, plaintext payload absent.
func seedOrderPaidJob(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	jobID := uuid.New()
	if err := db.Exec(`INSERT INTO notification_jobs (id, event_id, order_id, dedupe_key, channel, recipient, locale, template_key, payload_ciphertext, status, provider, dispatcher, type)
		VALUES (?, ?, ?, ?, 'email', 'buyer@example.com', 'en', 'order_paid', 'encrypted-render-data', 'pending', 'smtp', 'outbox', 'order_paid')`,
		jobID, uuid.New(), uuid.New(), uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
	return jobID
}

// seedScheduledJob goes through ScheduleEmail so the test exercises the real
// writer, which is where the ownership marker has to be set.
func seedScheduledJob(t *testing.T, db *gorm.DB, jobType string) uuid.UUID {
	t.Helper()
	repository := NewRepository(db).WithDefaultLocale("uk")
	err := db.Transaction(func(tx *gorm.DB) error {
		// Empty locale: the caller does not know the recipient's, so the
		// store's configured one must be stored rather than a hard-coded "en".
		return repository.ScheduleEmail(transaction.WithContext(context.Background(), tx), jobType, "", "buyer@example.com",
			map[string]string{"product": "Hand cream"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var jobID uuid.UUID
	if err := db.Raw(`SELECT id FROM notification_jobs WHERE type = ? ORDER BY created_at DESC LIMIT 1`, jobType).Row().Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	return jobID
}

func newNotificationTestRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("notifications_test"),
		postgresContainer.WithUsername("notifications"),
		postgresContainer.WithPassword("notifications"),
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
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	entries, err := os.ReadDir(filepath.Join(root, "migrations", "modules", "notifications"))
	if err != nil {
		t.Fatal(err)
	}
	// Applying the whole directory rather than a fixed list keeps this fixture
	// honest when a later migration changes the schema under it.
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, "migrations", "modules", "notifications", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(string(raw)).Error; err != nil {
			t.Fatalf("apply %s: %v", entry.Name(), err)
		}
	}
	return NewRepository(db), db
}

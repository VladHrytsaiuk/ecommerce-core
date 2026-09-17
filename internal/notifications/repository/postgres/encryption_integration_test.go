//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// A scheduled job used to keep the recipient and the rendered body in columns
// anyone with database access could read, and to concatenate both into an
// indexed dedupe key. These hold the shape that replaced it, and the dual-read
// that lets jobs queued by the old code still be sent.

const testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestAScheduledJobStoresNothingReadable(t *testing.T) {
	repository, db := newEncryptingRepository(t)

	scheduleOne(t, db, repository, "support_agent_reply", "buyer@example.test",
		map[string]string{"message_body": "your refund is on the way"})

	var row struct {
		RecipientEmail *string
		Payload        *string
		DedupeKey      string
		Ciphertext     string
	}
	if err := db.Raw(`SELECT recipient_email, payload, dedupe_key, payload_ciphertext AS ciphertext
		FROM notification_jobs WHERE type = 'support_agent_reply'`).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.RecipientEmail != nil || row.Payload != nil {
		t.Fatalf("the row still carries plaintext: email=%v payload=%v", row.RecipientEmail, row.Payload)
	}
	for _, secret := range []string{"buyer@example.test", "refund", "message_body"} {
		if strings.Contains(row.DedupeKey, secret) {
			t.Fatalf("the dedupe key contains %q: %s", secret, row.DedupeKey)
		}
		if strings.Contains(row.Ciphertext, secret) {
			t.Fatalf("the ciphertext is not encrypted; it contains %q", secret)
		}
	}
}

func TestTheWorkerReadsBackWhatItNeedsToSend(t *testing.T) {
	repository, db := newEncryptingRepository(t)
	scheduleOne(t, db, repository, "abandoned_cart", "Buyer@Example.test", map[string]int{"step": 2})

	job, err := repository.ClaimDue(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if job == nil {
		t.Fatal("nothing claimed")
	}
	if job.Email != "buyer@example.test" {
		t.Fatalf("Email = %q, want the normalised recipient out of the ciphertext", job.Email)
	}
	var payload struct {
		Step int `json:"step"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatalf("payload did not survive the round trip: %v", err)
	}
	if payload.Step != 2 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestAJobQueuedBeforeTheChangeIsStillSent(t *testing.T) {
	// The dual-read is what makes the rollout safe: rows written by the old
	// code are in the queue when the new code starts.
	repository, db := newEncryptingRepository(t)
	seedPlaintextJob(t, db, "back_in_stock", "legacy@example.test", `{"variant":"abc"}`)

	job, err := repository.ClaimDue(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("ClaimDue() error = %v", err)
	}
	if job == nil || job.Email != "legacy@example.test" || !strings.Contains(string(job.Payload), "abc") {
		t.Fatalf("claimed = %+v, want the plaintext row read as before", job)
	}
}

func TestTheSameMessageIsStillQueuedOnlyOnce(t *testing.T) {
	// The dedupe key changed shape; it must not have changed meaning.
	repository, db := newEncryptingRepository(t)
	for range 3 {
		scheduleOne(t, db, repository, "abandoned_cart", "buyer@example.test", map[string]int{"step": 1})
	}

	var queued int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_jobs WHERE type = 'abandoned_cart'`).Scan(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("%d jobs queued for one message, want 1", queued)
	}
}

func TestReencryptionEmptiesThePlaintextColumns(t *testing.T) {
	repository, db := newEncryptingRepository(t)
	for index := range 5 {
		seedPlaintextJob(t, db, "abandoned_cart", "old@example.test", `{"step":`+string(rune('0'+index))+`}`)
	}

	converted, err := repository.ReencryptPlaintext(context.Background(), 100)
	if err != nil {
		t.Fatalf("ReencryptPlaintext() error = %v", err)
	}
	if converted != 5 {
		t.Fatalf("converted %d jobs, want 5", converted)
	}
	remaining, err := repository.CountPlaintext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("%d jobs still in plaintext", remaining)
	}
	// And the dedupe keys those columns were concatenated into are gone too.
	var leaking int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_jobs WHERE dedupe_key LIKE '%@%'`).Scan(&leaking).Error; err != nil {
		t.Fatal(err)
	}
	if leaking != 0 {
		t.Fatalf("%d dedupe keys still contain an address", leaking)
	}
}

func TestReencryptionKeepsTheJobSendable(t *testing.T) {
	repository, db := newEncryptingRepository(t)
	seedPlaintextJob(t, db, "return_approved", "buyer@example.test", `{"return_id":"r-1"}`)

	if _, err := repository.ReencryptPlaintext(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	job, err := repository.ClaimDue(context.Background(), time.Now().UTC())
	if err != nil || job == nil {
		t.Fatalf("ClaimDue() = (%v, %v)", job, err)
	}
	if job.Email != "buyer@example.test" || !strings.Contains(string(job.Payload), "r-1") {
		t.Fatalf("the job lost what it needs to send: %+v", job)
	}
}

func TestReencryptionIsIdempotentAndBounded(t *testing.T) {
	repository, db := newEncryptingRepository(t)
	for index := range 4 {
		seedPlaintextJob(t, db, "abandoned_cart", "old@example.test", `{"step":`+string(rune('0'+index))+`}`)
	}

	first, err := repository.ReencryptPlaintext(context.Background(), 2)
	if err != nil || first != 2 {
		t.Fatalf("first batch = (%d, %v), want 2", first, err)
	}
	second, err := repository.ReencryptPlaintext(context.Background(), 100)
	if err != nil || second != 2 {
		t.Fatalf("second batch = (%d, %v), want the remaining 2", second, err)
	}
	third, err := repository.ReencryptPlaintext(context.Background(), 100)
	if err != nil || third != 0 {
		t.Fatalf("third batch = (%d, %v), want nothing left", third, err)
	}
}

func TestWritingIsRefusedWithoutEncryption(t *testing.T) {
	// A miswiring must not quietly go back to plaintext.
	_, db := newEncryptingRepository(t)
	bare := NewRepository(db).WithDefaultLocale("en")

	err := transaction.Within(context.Background(), db, func(tx *gorm.DB) error {
		return bare.ScheduleEmail(transaction.WithContext(context.Background(), tx), "abandoned_cart", "en", "buyer@example.test", map[string]int{"step": 1})
	})
	if err == nil {
		t.Fatal("ScheduleEmail() wrote a job with no encryption configured")
	}
}

func newEncryptingRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	repository, db := newTemplateRepository(t)
	cipher, err := encryption.NewAESGCM(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	dedupe, err := notifications.NewDedupeKeyer(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	return repository.WithDefaultLocale("en").WithEncryption(cipher, dedupe), db
}

func scheduleOne(t *testing.T, db *gorm.DB, repository *Repository, messageType, email string, payload any) {
	t.Helper()
	if err := transaction.Within(context.Background(), db, func(tx *gorm.DB) error {
		return repository.ScheduleEmail(transaction.WithContext(context.Background(), tx), messageType, "en", email, payload)
	}); err != nil {
		t.Fatal(err)
	}
}

// seedPlaintextJob writes a row the way the code did before this change.
func seedPlaintextJob(t *testing.T, db *gorm.DB, messageType, email, payload string) {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO notification_jobs
		(id, event_id, order_id, dedupe_key, channel, recipient_email, locale, template_key,
		 payload_ciphertext, payload, status, provider, dispatcher, type, retry_count, next_retry_at, created_at, updated_at)
		VALUES (?, ?, NULL, ?, 'email', ?, 'en', ?, '{}', ?, 'pending', 'scheduled', 'scheduler', ?, 0,
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, uuid.New(), messageType+":"+email+":"+payload, email, messageType, payload, messageType).Error; err != nil {
		t.Fatal(err)
	}
}

// withTestEncryption configures a repository the way the composition root does.
// ScheduleEmail refuses to write without it, deliberately: a miswiring must not
// quietly put a recipient back in the clear.
func withTestEncryption(t *testing.T, repository *Repository) *Repository {
	t.Helper()
	cipher, err := encryption.NewAESGCM(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	dedupe, err := notifications.NewDedupeKeyer(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	return repository.WithEncryption(cipher, dedupe)
}

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"time"
)

func (r *Repository) ScheduleEmail(ctx context.Context, typ, locale, email string, payload any) error {
	tx, err := transaction.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("notification schedule requires transaction: %w", err)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if typ == "" || email == "" {
		return fmt.Errorf("notification type and email are required")
	}
	// notification_jobs.locale is NOT NULL and rejects an empty string, and a
	// caller that does not know the recipient's language should get the
	// store's, not a language picked in this file.
	if locale = strings.ToLower(strings.TrimSpace(locale)); locale == "" {
		locale = r.defaultLocale
	}
	if locale == "" {
		return fmt.Errorf("notification schedule requires a locale or a configured default")
	}
	if r.cipher == nil || r.dedupe == nil {
		// Refuse rather than fall back to plaintext: a miswiring must not
		// quietly reintroduce the thing this replaced.
		return fmt.Errorf("notification schedule requires encryption to be configured")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// The recipient travels with the payload, so neither is a column anyone can
	// read. The worker decrypts this to address and render the message.
	sealed, err := json.Marshal(scheduledPayload{Email: email, Payload: raw})
	if err != nil {
		return err
	}
	ciphertext, err := r.cipher.Encrypt(sealed)
	if err != nil {
		return fmt.Errorf("encrypt scheduled notification payload: %w", err)
	}
	// An HMAC over the same three values the old key concatenated: it
	// deduplicates identically and reveals nothing, where the old one put the
	// address and the rendered body in an indexed column.
	dedupe, err := r.dedupe.Key(typ, email, raw)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	// dispatcher names the worker that owns this row. Without it DurableWorker
	// also claimed the order-paid receipts, whose render data is encrypted and
	// whose payload column is NULL, and recorded them dead.
	return tx.Exec(`INSERT INTO notification_jobs (id,event_id,order_id,dedupe_key,channel,recipient_email,locale,template_key,payload_ciphertext,status,provider,dispatcher,type,payload,retry_count,next_retry_at,created_at) VALUES (?,?,NULL,?,'email',NULL,?,?,?,'pending','scheduled','scheduler',?,NULL,0,?,?) ON CONFLICT (dedupe_key) DO NOTHING`, uuid.New(), uuid.New(), dedupe, locale, typ, ciphertext, typ, now, now).Error
}

// scheduledPayload is what a scheduled job's ciphertext holds: everything the
// worker needs to send, and nothing the database needs to index.
type scheduledPayload struct {
	Email   string          `json:"email"`
	Payload json.RawMessage `json:"payload"`
}

var _ notifications.NotificationScheduler = (*Repository)(nil)

// ClaimDue takes the next scheduled job that is due, or one whose lease has
// expired. It claims only rows this worker owns: the order-paid receipts in
// the same table belong to the outbox handler, which claims them by id.
func (r *Repository) ClaimDue(ctx context.Context, now time.Time) (*notifications.DurableJob, error) {
	var row struct {
		ID             uuid.UUID
		Type           string
		RecipientEmail string
		Locale         string
		Payload        []byte
		Ciphertext     string
		RetryCount     int
		LockToken      uuid.UUID
	}
	token := uuid.New()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(`WITH c AS (SELECT id FROM notification_jobs WHERE dispatcher='scheduler' AND ((status IN ('pending','failed') AND next_retry_at<=?) OR (status='sending' AND locked_at<=?)) ORDER BY next_retry_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE notification_jobs j SET status='sending',retry_count=j.retry_count+1,locked_at=?,lock_token=?,updated_at=CURRENT_TIMESTAMP FROM c WHERE j.id=c.id RETURNING j.id,j.type,j.recipient_email,j.locale,j.payload,j.payload_ciphertext AS ciphertext,j.retry_count,j.lock_token`, now, now.Add(-5*time.Minute), now, token).Scan(&row).Error
	})
	if err != nil {
		return nil, err
	}
	if row.ID == uuid.Nil {
		return nil, nil
	}
	// Dual-read for the length of the rollout. A row written before the change
	// carries its recipient and payload in the clear; one written after carries
	// them in the ciphertext and nothing in those columns. Plaintext is
	// preferred when present so a job queued by the old code is still sent, and
	// stage 3 removes the last of them.
	email, payload := row.RecipientEmail, row.Payload
	if payload == nil {
		sealed, err := r.openScheduledPayload(row.Ciphertext)
		if err != nil {
			return nil, fmt.Errorf("open scheduled notification payload: %w", err)
		}
		email, payload = sealed.Email, sealed.Payload
	}
	return &notifications.DurableJob{ID: row.ID, LockToken: row.LockToken, Type: row.Type, Email: email, Locale: row.Locale, Payload: payload, RetryCount: row.RetryCount}, nil
}

func (r *Repository) openScheduledPayload(ciphertext string) (scheduledPayload, error) {
	if r.cipher == nil {
		return scheduledPayload{}, fmt.Errorf("notification encryption is not configured")
	}
	plaintext, err := r.cipher.Decrypt(ciphertext)
	if err != nil {
		return scheduledPayload{}, err
	}
	var sealed scheduledPayload
	if err := json.Unmarshal(plaintext, &sealed); err != nil {
		return scheduledPayload{}, err
	}
	if strings.TrimSpace(sealed.Email) == "" || len(sealed.Payload) == 0 {
		return scheduledPayload{}, fmt.Errorf("scheduled notification payload is incomplete")
	}
	return sealed, nil
}
func (r *Repository) Complete(ctx context.Context, job notifications.DurableJob) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE notification_jobs SET status='sent',sent_at=CURRENT_TIMESTAMP,locked_at=NULL,lock_token=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='sending' AND lock_token=?`, job.ID, job.LockToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("notification job lease lost")
	}
	return nil
}
func (r *Repository) Fail(ctx context.Context, job notifications.DurableJob, next time.Time, dead bool) error {
	status := "failed"
	if dead {
		status = "dead"
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE notification_jobs SET status=?,next_retry_at=?,locked_at=NULL,lock_token=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='sending' AND lock_token=?`, status, next, job.ID, job.LockToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("notification job lease lost")
	}
	return nil
}

var _ notifications.DurableJobStore = (*Repository)(nil)

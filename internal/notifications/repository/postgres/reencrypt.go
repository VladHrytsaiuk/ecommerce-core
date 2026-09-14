package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReencryptPlaintext moves one bounded batch of scheduled jobs from plaintext
// columns into the ciphertext, and rewrites their dedupe key.
//
// It runs through the application because the encryption key lives here: a SQL
// backfill cannot produce ciphertext, and a migration that tried would have to
// be handed the key.
//
// Terminal rows are included deliberately. A sent job keeps its recipient for
// as long as retention holds it, so leaving those in the clear would mean the
// plaintext columns could not be dropped for another thirty days — and the
// point of this is that they stop existing.
func (r *Repository) ReencryptPlaintext(ctx context.Context, limit int) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("notification repository is not configured")
	}
	if r.cipher == nil || r.dedupe == nil {
		return 0, fmt.Errorf("re-encryption requires the encryption key")
	}
	if limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("re-encryption batch must be between 1 and 10000")
	}
	var converted int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			ID             uuid.UUID
			Type           string
			RecipientEmail string
			Payload        []byte
		}
		// SKIP LOCKED so this never waits on the dispatcher, and never takes a
		// row it is sending.
		if err := tx.Raw(`
			SELECT id, type, recipient_email, payload
			FROM notification_jobs
			WHERE dispatcher = 'scheduler' AND payload IS NOT NULL
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT ?`, limit).Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			email := strings.ToLower(strings.TrimSpace(row.RecipientEmail))
			if email == "" {
				return fmt.Errorf("notification job %s has no recipient to encrypt", row.ID)
			}
			sealed, err := json.Marshal(scheduledPayload{Email: email, Payload: row.Payload})
			if err != nil {
				return err
			}
			ciphertext, err := r.cipher.Encrypt(sealed)
			if err != nil {
				return fmt.Errorf("encrypt notification job %s: %w", row.ID, err)
			}
			dedupe, err := r.dedupe.Key(row.Type, email, row.Payload)
			if err != nil {
				return fmt.Errorf("key notification job %s: %w", row.ID, err)
			}
			// The old dedupe key held the address and the rendered body, so
			// rewriting it is part of the job rather than a tidy-up. A
			// collision means an identical message is already queued under the
			// new key; this row is the duplicate, and deleting it is what the
			// original ON CONFLICT would have done.
			result := tx.Exec(`
				UPDATE notification_jobs
				SET payload_ciphertext = ?, dedupe_key = ?, payload = NULL, recipient_email = NULL,
				    updated_at = updated_at
				WHERE id = ? AND NOT EXISTS (
				    SELECT 1 FROM notification_jobs other WHERE other.dedupe_key = ? AND other.id <> ?
				)`, ciphertext, dedupe, row.ID, dedupe, row.ID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				if err := tx.Exec(`DELETE FROM notification_jobs WHERE id = ?`, row.ID).Error; err != nil {
					return err
				}
			}
			converted++
		}
		return nil
	})
	return converted, err
}

// CountPlaintext reports how many scheduled jobs still hold their recipient and
// payload in the clear. Zero is the precondition for dropping those columns.
func (r *Repository) CountPlaintext(ctx context.Context) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("notification repository is not configured")
	}
	var remaining int64
	if err := r.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM notification_jobs WHERE dispatcher = 'scheduler' AND payload IS NOT NULL`).Scan(&remaining).Error; err != nil {
		return 0, err
	}
	return int(remaining), nil
}

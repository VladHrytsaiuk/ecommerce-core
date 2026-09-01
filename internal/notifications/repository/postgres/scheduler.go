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

func (r *Repository) ScheduleEmail(ctx context.Context, typ, email string, payload any) error {
	tx, err := transaction.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("notification schedule requires transaction: %w", err)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if typ == "" || email == "" {
		return fmt.Errorf("notification type and email are required")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	dedupe := typ + ":" + email + ":" + string(raw)
	now := time.Now().UTC()
	return tx.Exec(`INSERT INTO notification_jobs (id,event_id,order_id,dedupe_key,channel,recipient_email,locale,template_key,payload_ciphertext,status,provider,type,payload,retry_count,next_retry_at,created_at) VALUES (?,?,NULL,?,'email',?,'en',?,'{}','pending','scheduled',?,?,0,?,?) ON CONFLICT (dedupe_key) DO NOTHING`, uuid.New(), uuid.New(), dedupe, email, typ, typ, string(raw), now, now).Error
}

var _ notifications.NotificationScheduler = (*Repository)(nil)

func (r *Repository) ClaimDue(ctx context.Context, now time.Time) (*notifications.DurableJob, error) {
	var row struct {
		ID             uuid.UUID
		Type           string
		RecipientEmail string
		Payload        []byte
		RetryCount     int
		LockToken      uuid.UUID
	}
	token := uuid.New()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(`WITH c AS (SELECT id FROM notification_jobs WHERE (status IN ('pending','failed') AND next_retry_at<=?) OR (status='sending' AND locked_at<=?) ORDER BY next_retry_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE notification_jobs j SET status='sending',retry_count=j.retry_count+1,locked_at=?,lock_token=?,updated_at=CURRENT_TIMESTAMP FROM c WHERE j.id=c.id RETURNING j.id,j.type,j.recipient_email,j.payload,j.retry_count,j.lock_token`, now, now.Add(-5*time.Minute), now, token).Scan(&row).Error
	})
	if err != nil {
		return nil, err
	}
	if row.ID == uuid.Nil {
		return nil, nil
	}
	return &notifications.DurableJob{ID: row.ID, LockToken: row.LockToken, Type: row.Type, Email: row.RecipientEmail, Payload: row.Payload, RetryCount: row.RetryCount}, nil
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

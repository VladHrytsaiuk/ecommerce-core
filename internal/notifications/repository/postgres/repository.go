// Package postgres implements Notifications persistence without importing an
// email provider. Core order contacts are read through the module's narrow
// repository port and are never joined into checkout writes.
package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/sanitize"
)

type Repository struct {
	db            *gorm.DB
	defaultLocale string
}

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// WithDefaultLocale supplies the store's locale for scheduled jobs whose
// caller does not know the recipient's.
func (r *Repository) WithDefaultLocale(locale string) *Repository {
	if r != nil {
		r.defaultLocale = strings.ToLower(strings.TrimSpace(locale))
	}
	return r
}

type contactRecord struct {
	OrderID uuid.UUID
	Email   string
	Locale  string
}

func (contactRecord) TableName() string { return "order_contact_details" }

type jobRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	EventID           uuid.UUID
	OrderID           uuid.UUID
	DedupeKey         string
	Channel           string
	Recipient         *string
	Locale            string
	TemplateKey       string
	PayloadCiphertext string
	Status            string
	Provider          string
	Dispatcher        string
	ProviderMessageID *string
	Attempts          int
	LockedAt          *time.Time
	LockToken         *uuid.UUID
	LastError         *string
	SentAt            *time.Time
	CreatedAt         time.Time
}

func (jobRecord) TableName() string { return "notification_jobs" }

type attemptRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	JobID             uuid.UUID
	AttemptNumber     int
	Provider          string
	ProviderMessageID *string
	Status            string
	ErrorCode         *string
	CompletedAt       time.Time
}

type templateRecord struct {
	TemplateKey     string
	Locale          string
	SubjectTemplate string
	HTMLTemplate    string
	TextTemplate    string
}

func (templateRecord) TableName() string { return "notification_templates" }

func (attemptRecord) TableName() string { return "notification_attempts" }

func (r *Repository) FindOrderContact(ctx context.Context, orderID uuid.UUID) (*notificationsDomain.OrderContact, error) {
	var record contactRecord
	if err := r.db.WithContext(ctx).First(&record, "order_id = ?", orderID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &notificationsDomain.OrderContact{OrderID: record.OrderID, Email: record.Email, Locale: record.Locale}, nil
}

func (r *Repository) FindTemplate(ctx context.Context, key, locale, fallbackLocale string) (*notificationsDomain.Template, error) {
	find := func(requestedLocale string) (*notificationsDomain.Template, error) {
		var record templateRecord
		if err := r.db.WithContext(ctx).Where("template_key = ? AND channel = ? AND locale = ? AND is_active = true", key, "email", requestedLocale).First(&record).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, nil
			}
			return nil, err
		}
		return &notificationsDomain.Template{Key: record.TemplateKey, Locale: record.Locale, Subject: record.SubjectTemplate, HTML: record.HTMLTemplate, Text: record.TextTemplate}, nil
	}
	template, err := find(locale)
	if err != nil || template != nil || locale == fallbackLocale {
		return template, err
	}
	return find(fallbackLocale)
}

func (r *Repository) CreateOrderPaidJob(ctx context.Context, job notificationsDomain.Job) (*notificationsDomain.Job, bool, error) {
	if strings.TrimSpace(job.PayloadCiphertext) == "" {
		return nil, false, fmt.Errorf("notification job payload ciphertext is required")
	}
	record := jobRecord{ID: uuid.New(), EventID: job.EventID, OrderID: job.OrderID, DedupeKey: job.DedupeKey, Channel: "email", Locale: job.Locale, TemplateKey: notificationsDomain.OrderPaidTemplate, PayloadCiphertext: job.PayloadCiphertext, Status: "pending", Provider: job.Provider, Dispatcher: "outbox"}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&record)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return toDomainJob(record), true, nil
	}
	if err := r.db.WithContext(ctx).Where("dedupe_key = ?", job.DedupeKey).First(&record).Error; err != nil {
		return nil, false, err
	}
	return toDomainJob(record), false, nil
}

func (r *Repository) ClaimOrderPaidJob(ctx context.Context, jobID uuid.UUID, now time.Time, lease time.Duration) (*notificationsDomain.Job, bool, error) {
	if lease <= 0 {
		return nil, false, fmt.Errorf("notification job lease must be positive")
	}
	token := uuid.New()
	var record jobRecord
	result := r.db.WithContext(ctx).Raw(`UPDATE notification_jobs
SET status = 'sending', attempts = attempts + 1, locked_at = ?, lock_token = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
  AND dispatcher = 'outbox'
  AND (status IN ('pending', 'failed') OR (status = 'sending' AND locked_at <= ?))
RETURNING id, event_id, order_id, dedupe_key, channel, recipient, locale, template_key, payload_ciphertext, status, provider, dispatcher, provider_message_id, attempts, locked_at, lock_token, last_error, sent_at, created_at`, now, token, jobID, now.Add(-lease)).Scan(&record)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	return toDomainJob(record), true, nil
}

func (r *Repository) RecordAttempt(ctx context.Context, claimed notificationsDomain.Job, provider string, receipt notificationsDomain.DeliveryReceipt, sendErr error, completedAt time.Time, maxAttempts int) error {
	if claimed.ID == uuid.Nil || claimed.LockToken == nil || maxAttempts <= 0 {
		return fmt.Errorf("invalid notification job attempt")
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		var job jobRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ?", claimed.ID).Error; err != nil {
			return err
		}
		if job.Status != "sending" || job.LockToken == nil || *job.LockToken != *claimed.LockToken {
			return fmt.Errorf("notification job is no longer claimed")
		}
		status := "success"
		jobStatus := "sent"
		var providerMessageID *string
		if receipt.ProviderMessageID != "" {
			providerMessageID = &receipt.ProviderMessageID
		}
		var errorCode *string
		if sendErr != nil {
			status = "failed"
			code := sanitize.ErrorCode(sendErr)
			errorCode = &code
			jobStatus = "pending"
			if job.Attempts >= maxAttempts {
				jobStatus = "dead"
			}
		}
		attempt := attemptRecord{ID: uuid.New(), JobID: job.ID, AttemptNumber: job.Attempts, Provider: provider, ProviderMessageID: providerMessageID, Status: status, ErrorCode: errorCode, CompletedAt: completedAt}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		updates := map[string]any{"status": jobStatus, "provider_message_id": providerMessageID, "last_error": errorCode, "locked_at": nil, "lock_token": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
		if sendErr == nil {
			updates["sent_at"] = completedAt
		}
		return tx.Model(&job).Updates(updates).Error
	})
}

func toDomainJob(record jobRecord) *notificationsDomain.Job {
	return &notificationsDomain.Job{ID: record.ID, EventID: record.EventID, OrderID: record.OrderID, DedupeKey: record.DedupeKey, Locale: record.Locale, Provider: record.Provider, PayloadCiphertext: record.PayloadCiphertext, Status: record.Status, Attempts: record.Attempts, LockedAt: record.LockedAt, LockToken: record.LockToken, CreatedAt: record.CreatedAt}
}

var _ notificationsDomain.Repository = (*Repository)(nil)

// SynchronizeTemplates installs the shipped defaults for one locale, leaving
// anything already there untouched.
//
// A migration could not do this: templates are keyed by locale and the store's
// fallback locale is configuration, not schema. It is the same shape as the
// locale synchroniser, and for the same reason.
func (r *Repository) SynchronizeTemplates(ctx context.Context, locale string, templates []notificationsDomain.Template) error {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" {
		return fmt.Errorf("notification template locale is required")
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		for _, template := range templates {
			// DO NOTHING, never an update: an operator who rewrote a template
			// must not find this core's default back in place after a deploy.
			// Untargeted, so it covers both the (key, channel, locale, version)
			// constraint and the partial unique index over active rows.
			if err := tx.Exec(`
				INSERT INTO notification_templates
					(id, template_key, channel, locale, version, subject_template, html_template, text_template, is_active)
				VALUES (?, ?, 'email', ?, 1, ?, ?, ?, true)
				ON CONFLICT DO NOTHING`,
				uuid.New(), template.Key, locale, template.Subject, template.HTML, template.Text).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var _ notificationsDomain.TemplateSynchronizer = (*Repository)(nil)

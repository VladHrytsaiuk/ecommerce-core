package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

type WebhookEventStore struct{ db *gorm.DB }

func NewWebhookEventStore(db *gorm.DB) *WebhookEventStore { return &WebhookEventStore{db: db} }

type webhookEventRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	Provider          string
	EventID           string
	OrderID           uuid.UUID
	ProviderReference string
	EventStatus       string
	Amount            int64
	Currency          string
	ProcessingStatus  string
}

func (webhookEventRecord) TableName() string { return "payment_webhook_events" }

// webhookLease bounds how long one replica may hold an unfinished callback.
// A provider retry arriving inside this window is refused; after it, a crashed
// replica's work is safely taken over.
const webhookLease = 30 * time.Second

// Claim takes exclusive ownership of a provider callback.
//
// The previous implementation fell back to reading processing_status without a
// lock, so two replicas handed the same provider retry could both observe
// 'processing' and run the callback concurrently. The order workflow is
// idempotent under row locks, so no money was ever at risk, but the losing
// replica reported ErrInvalidOrderTransition and that surfaced as a payment
// processing failure that had not actually occurred.
//
// A single statement now inserts a new claim or takes over one whose lease has
// expired, which keeps crash recovery working without permitting two live
// holders. A fully processed event never matches and remains a no-op.
func (s *WebhookEventStore) Claim(ctx context.Context, provider string, event paymentsDomain.PaymentEvent) (bool, error) {
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO payment_webhook_events
		       (id, provider, event_id, order_id, provider_reference,
		        event_status, amount, currency, processing_status, locked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'processing', CURRENT_TIMESTAMP)
		ON CONFLICT (provider, event_id) DO UPDATE
		   SET locked_at = CURRENT_TIMESTAMP
		 WHERE payment_webhook_events.processing_status = 'processing'
		   AND payment_webhook_events.locked_at < CURRENT_TIMESTAMP - CAST(? AS INTERVAL)`,
		uuid.New(), provider, event.EventID, event.OrderID, event.ProviderReference,
		event.Status, event.Amount.Amount(), event.Amount.Currency(),
		strconv.Itoa(int(webhookLease.Seconds()))+" seconds")
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (s *WebhookEventStore) MarkProcessed(ctx context.Context, provider, eventID string) error {
	return s.db.WithContext(ctx).Model(&webhookEventRecord{}).Where("provider = ? AND event_id = ? AND processing_status = ?", provider, eventID, "processing").Updates(map[string]any{"processing_status": "processed", "processed_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}

func (s *WebhookEventStore) Abandon(ctx context.Context, provider, eventID string) error {
	return s.db.WithContext(ctx).Where("provider = ? AND event_id = ? AND processing_status = ?", provider, eventID, "processing").Delete(&webhookEventRecord{}).Error
}

var _ paymentsDomain.WebhookEventStore = (*WebhookEventStore)(nil)

package postgres

import (
	"context"
	"fmt"
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
	LockToken         *uuid.UUID
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
func (s *WebhookEventStore) Claim(ctx context.Context, provider string, event paymentsDomain.PaymentEvent) (paymentsDomain.WebhookClaim, bool, error) {
	token := uuid.New()
	result := s.db.WithContext(ctx).Exec(`
		INSERT INTO payment_webhook_events
		       (id, provider, event_id, order_id, provider_reference,
		        event_status, amount, currency, processing_status, locked_at, lock_token)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'processing', CURRENT_TIMESTAMP, ?)
		ON CONFLICT (provider, event_id) DO UPDATE
		   SET locked_at = CURRENT_TIMESTAMP, lock_token = EXCLUDED.lock_token
		 WHERE payment_webhook_events.processing_status = 'processing'
		   AND payment_webhook_events.locked_at < CURRENT_TIMESTAMP - CAST(? AS INTERVAL)`,
		uuid.New(), provider, event.EventID, event.OrderID, event.ProviderReference,
		event.Status, event.Amount.Amount(), event.Amount.Currency(), token,
		strconv.Itoa(int(webhookLease.Seconds()))+" seconds")
	if result.Error != nil {
		return paymentsDomain.WebhookClaim{}, false, result.Error
	}
	if result.RowsAffected != 1 {
		return paymentsDomain.WebhookClaim{}, false, nil
	}
	return paymentsDomain.WebhookClaim{Provider: provider, EventID: event.EventID, LockToken: token}, true, nil
}

// MarkProcessed closes the callback this replica claimed. A claim that was
// taken over in the meantime matches nothing, which is correct: the replica
// that holds it now owns the outcome.
func (s *WebhookEventStore) MarkProcessed(ctx context.Context, claim paymentsDomain.WebhookClaim) error {
	return s.finalize(ctx, claim, func(query *gorm.DB) *gorm.DB {
		return query.Updates(map[string]any{"processing_status": "processed", "processed_at": gorm.Expr("CURRENT_TIMESTAMP"), "lock_token": nil})
	})
}

// Abandon removes the deduplication record so the provider's next retry is
// handled afresh. It is gated on the token for the same reason as
// MarkProcessed, and more urgently: deleting the row a live replica holds
// would erase the record that replica is working under.
func (s *WebhookEventStore) Abandon(ctx context.Context, claim paymentsDomain.WebhookClaim) error {
	return s.finalize(ctx, claim, func(query *gorm.DB) *gorm.DB {
		return query.Delete(&webhookEventRecord{})
	})
}

func (s *WebhookEventStore) finalize(ctx context.Context, claim paymentsDomain.WebhookClaim, apply func(*gorm.DB) *gorm.DB) error {
	if claim.LockToken == uuid.Nil {
		return fmt.Errorf("payment webhook claim carries no lock token")
	}
	query := s.db.WithContext(ctx).Model(&webhookEventRecord{}).
		Where("provider = ? AND event_id = ? AND processing_status = ? AND lock_token = ?",
			claim.Provider, claim.EventID, "processing", claim.LockToken)
	return apply(query).Error
}

var _ paymentsDomain.WebhookEventStore = (*WebhookEventStore)(nil)

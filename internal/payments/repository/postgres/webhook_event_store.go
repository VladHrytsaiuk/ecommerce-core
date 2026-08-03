package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

func (s *WebhookEventStore) Claim(ctx context.Context, provider string, event paymentsDomain.PaymentEvent) (bool, error) {
	record := webhookEventRecord{ID: uuid.New(), Provider: provider, EventID: event.EventID, OrderID: event.OrderID, ProviderReference: event.ProviderReference, EventStatus: event.Status, Amount: event.Amount.Amount, Currency: event.Amount.Currency, ProcessingStatus: "processing"}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "provider"}, {Name: "event_id"}}, DoNothing: true}).Create(&record)
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

// Package events implements the transactional event outbox with PostgreSQL.
package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Publisher struct{ consumers []string }

func NewPublisher(consumers ...string) *Publisher {
	seen := make(map[string]struct{}, len(consumers))
	active := make([]string, 0, len(consumers))
	for _, consumer := range consumers {
		consumer = strings.TrimSpace(consumer)
		if consumer == "" {
			continue
		}
		if _, exists := seen[consumer]; exists {
			continue
		}
		seen[consumer] = struct{}{}
		active = append(active, consumer)
	}
	return &Publisher{consumers: active}
}

type eventRecord struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Topic          string
	AggregateType  string
	AggregateID    uuid.UUID
	IdempotencyKey uuid.UUID
	Payload        string
	OccurredAt     time.Time
	TraceParent    string `gorm:"column:traceparent"`
	TraceState     string `gorm:"column:tracestate"`
	RequestID      string `gorm:"column:request_id"`
}

func (eventRecord) TableName() string { return "domain_events" }

type deliveryRecord struct {
	EventID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	Consumer    string    `gorm:"primaryKey"`
	Status      string
	Attempts    int
	AvailableAt time.Time
	LockedAt    *time.Time
	CompletedAt *time.Time
	LastError   *string
}

func (deliveryRecord) TableName() string { return "event_deliveries" }

func (p *Publisher) Publish(ctx context.Context, event eventsDomain.DomainEvent) error {
	if p == nil || event == nil {
		return fmt.Errorf("event publisher and event are required")
	}
	tx, err := transaction.FromContext(ctx)
	if err != nil {
		return err
	}
	ctx, span := observability.StartOutboxPublish(ctx, event.Topic())
	defer span.End()
	payload, err := event.MarshalPayload()
	if err != nil {
		return fmt.Errorf("marshal %s event: %w", event.Topic(), err)
	}
	if event.AggregateID() == uuid.Nil || event.IdempotencyKey() == uuid.Nil || strings.TrimSpace(event.Topic()) == "" || strings.TrimSpace(event.AggregateType()) == "" || len(payload) == 0 {
		return fmt.Errorf("invalid domain event")
	}
	occurredAt := event.OccurredAt()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	traceContext := observability.CaptureOutboxTraceContext(ctx)
	record := eventRecord{ID: uuid.New(), Topic: event.Topic(), AggregateType: event.AggregateType(), AggregateID: event.AggregateID(), IdempotencyKey: event.IdempotencyKey(), Payload: string(payload), OccurredAt: occurredAt, TraceParent: traceContext.TraceParent, TraceState: traceContext.TraceState, RequestID: traceContext.RequestID}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "topic"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	if len(p.consumers) == 0 {
		return nil
	}
	deliveries := make([]deliveryRecord, 0, len(p.consumers))
	for _, consumer := range p.consumers {
		deliveries = append(deliveries, deliveryRecord{EventID: record.ID, Consumer: consumer, Status: "pending", AvailableAt: occurredAt})
	}
	return tx.Create(&deliveries).Error
}

type DeliveryStore struct{ db *gorm.DB }

func NewDeliveryStore(db *gorm.DB) *DeliveryStore { return &DeliveryStore{db: db} }

type claimedDelivery struct {
	EventID       uuid.UUID
	Topic         string
	AggregateType string
	AggregateID   uuid.UUID
	Payload       string
	Consumer      string
	Attempts      int
	OccurredAt    time.Time
	TraceParent   string `gorm:"column:traceparent"`
	TraceState    string `gorm:"column:tracestate"`
	RequestID     string `gorm:"column:request_id"`
}

// Claim obtains one due row without blocking another worker that is processing
// a different event. Expired leases are safely reclaimed.
func (s *DeliveryStore) Claim(ctx context.Context, consumer string, now time.Time, lease time.Duration) (*eventsDomain.Delivery, error) {
	if s == nil || s.db == nil || strings.TrimSpace(consumer) == "" || lease <= 0 {
		return nil, fmt.Errorf("invalid event delivery claim")
	}
	var record claimedDelivery
	query := `
WITH candidate AS (
    SELECT event_id, consumer
    FROM event_deliveries
    WHERE consumer = ?
      AND ((status IN ('pending', 'failed') AND available_at <= ?)
        OR (status = 'processing' AND locked_at <= ?))
    ORDER BY available_at, created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE event_deliveries AS delivery
SET status = 'processing', attempts = delivery.attempts + 1, locked_at = ?, updated_at = CURRENT_TIMESTAMP
FROM candidate, domain_events AS event
WHERE delivery.event_id = candidate.event_id
  AND delivery.consumer = candidate.consumer
  AND event.id = delivery.event_id
RETURNING delivery.event_id, event.topic, event.aggregate_type, event.aggregate_id, event.payload, delivery.consumer, delivery.attempts, event.occurred_at, event.traceparent, event.tracestate, event.request_id`
	result := s.db.WithContext(ctx).Raw(query, consumer, now, now.Add(-lease), now).Scan(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &eventsDomain.Delivery{EventID: record.EventID, Topic: record.Topic, AggregateType: record.AggregateType, AggregateID: record.AggregateID, Payload: []byte(record.Payload), Consumer: record.Consumer, Attempts: record.Attempts, OccurredAt: record.OccurredAt, TraceParent: record.TraceParent, TraceState: record.TraceState, RequestID: record.RequestID}, nil
}

func (s *DeliveryStore) Complete(ctx context.Context, eventID uuid.UUID, consumer string, completedAt time.Time) error {
	result := s.db.WithContext(ctx).Exec(`UPDATE event_deliveries SET status = 'done', completed_at = ?, locked_at = NULL, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE event_id = ? AND consumer = ? AND status = 'processing'`, completedAt, eventID, consumer)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("event delivery %s/%s is not claimed", eventID, consumer)
	}
	return nil
}

func (s *DeliveryStore) Fail(ctx context.Context, eventID uuid.UUID, consumer string, cause error, availableAt time.Time) error {
	if cause == nil {
		return fmt.Errorf("event delivery failure requires a cause")
	}
	result := s.db.WithContext(ctx).Exec(`UPDATE event_deliveries SET status = 'failed', available_at = ?, locked_at = NULL, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE event_id = ? AND consumer = ? AND status = 'processing'`, availableAt, cause.Error(), eventID, consumer)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("event delivery %s/%s is not claimed", eventID, consumer)
	}
	return nil
}

func (s *DeliveryStore) Dead(ctx context.Context, eventID uuid.UUID, consumer string, cause error, completedAt time.Time) error {
	if cause == nil {
		return fmt.Errorf("event delivery failure requires a cause")
	}
	result := s.db.WithContext(ctx).Exec(`UPDATE event_deliveries SET status = 'dead', completed_at = ?, locked_at = NULL, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE event_id = ? AND consumer = ? AND status = 'processing'`, completedAt, cause.Error(), eventID, consumer)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("event delivery %s/%s is not claimed", eventID, consumer)
	}
	return nil
}

var _ eventsDomain.TransactionalEventPublisher = (*Publisher)(nil)
var _ eventsDomain.DeliveryStore = (*DeliveryStore)(nil)

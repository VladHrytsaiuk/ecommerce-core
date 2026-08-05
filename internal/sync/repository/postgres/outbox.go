// Package postgres implements Sync persistence without importing an ERP SDK.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

type OutboxStore struct{ db *gorm.DB }

func NewOutboxStore(db *gorm.DB) *OutboxStore { return &OutboxStore{db: db} }

type eventRecord struct {
	ID             uuid.UUID
	Topic          string
	AggregateID    uuid.UUID
	IdempotencyKey uuid.UUID
	Payload        string
	Attempts       int
	CreatedAt      time.Time
}

func (eventRecord) TableName() string { return "sync_outbox" }

// Claim uses FOR UPDATE SKIP LOCKED so multiple API instances can process the
// shared outbox without delivering the same leased row concurrently.
func (s *OutboxStore) Claim(ctx context.Context, now time.Time, lease time.Duration) (*domain.OutboxEvent, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("sync outbox lease must be positive")
	}
	var event eventRecord
	query := `
WITH candidate AS (
    SELECT id
    FROM sync_outbox
    WHERE (status IN ('pending', 'failed') AND available_at <= ?)
       OR (status = 'processing' AND locked_at <= ?)
    ORDER BY available_at, created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE sync_outbox AS outbox
SET status = 'processing', attempts = outbox.attempts + 1, locked_at = ?, updated_at = CURRENT_TIMESTAMP
FROM candidate
WHERE outbox.id = candidate.id
RETURNING outbox.id, outbox.topic, outbox.aggregate_id, outbox.idempotency_key, outbox.payload, outbox.attempts, outbox.created_at`
	result := s.db.WithContext(ctx).Raw(query, now, now.Add(-lease), now).Scan(&event)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &domain.OutboxEvent{
		ID: event.ID, Topic: event.Topic, AggregateID: event.AggregateID,
		IdempotencyKey: event.IdempotencyKey, Payload: []byte(event.Payload), Attempts: event.Attempts, CreatedAt: event.CreatedAt,
	}, nil
}

var _ domain.OutboxStore = (*OutboxStore)(nil)

func (s *OutboxStore) Complete(ctx context.Context, eventID uuid.UUID, deliveredAt time.Time) error {
	result := s.db.WithContext(ctx).Exec(`UPDATE sync_outbox SET status = 'delivered', delivered_at = ?, locked_at = NULL, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'processing'`, deliveredAt, eventID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("sync outbox event %s is not claimed", eventID)
	}
	return nil
}

func (s *OutboxStore) Retry(ctx context.Context, eventID uuid.UUID, cause error, availableAt time.Time) error {
	if cause == nil {
		return fmt.Errorf("sync outbox retry requires a cause")
	}
	result := s.db.WithContext(ctx).Exec(`UPDATE sync_outbox SET status = 'failed', available_at = ?, locked_at = NULL, last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'processing'`, availableAt, cause.Error(), eventID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("sync outbox event %s is not claimed", eventID)
	}
	return nil
}

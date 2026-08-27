package events

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RetentionStore moves only successfully completed deliveries out of the hot
// claim table. Dead letters deliberately stay queryable for manual recovery.
type RetentionStore struct{ db *gorm.DB }

func NewRetentionStore(db *gorm.DB) *RetentionStore { return &RetentionStore{db: db} }

func (s *RetentionStore) ArchiveDone(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	if s == nil || s.db == nil || olderThan.IsZero() || limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("invalid outbox retention request")
	}
	var archived int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`
WITH candidates AS (
    SELECT event_id, consumer, status, attempts, completed_at, last_error
    FROM event_deliveries
    WHERE status = 'done' AND completed_at < ?
    ORDER BY completed_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT ?
), copied AS (
    INSERT INTO event_delivery_archive (event_id, consumer, status, attempts, completed_at, last_error)
    SELECT event_id, consumer, status, attempts, completed_at, last_error FROM candidates
    ON CONFLICT (event_id, consumer) DO NOTHING
)
DELETE FROM event_deliveries AS delivery
USING candidates
WHERE delivery.event_id = candidates.event_id
  AND delivery.consumer = candidates.consumer`, olderThan, limit)
		if result.Error != nil {
			return result.Error
		}
		archived = result.RowsAffected
		return nil
	})
	return int(archived), err
}

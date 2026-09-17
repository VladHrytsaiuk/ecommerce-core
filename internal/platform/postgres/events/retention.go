package events

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RetentionStore ages terminal delivery history out of the database in two
// stages: completed deliveries leave the hot claim table for the archive, and
// the archive itself is eventually emptied. Dead letters deliberately stay in
// the hot table, queryable for manual recovery.
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

// PruneArchive deletes archived delivery history past its window.
//
// Without it "retention" only ever moved rows between two tables and the
// database grew exactly as fast as before: event_delivery_archive had no
// reader, no window and no delete anywhere in the codebase. The archive answers
// "what happened to this delivery" weeks after the fact, which is what
// OUTBOX_ARCHIVE_RETENTION bounds.
//
// The window is measured from completed_at, not archived_at: an operator
// configures how long a delivery's outcome is retained, and that clock starts
// when the delivery finished. It is also the column the archive is already
// indexed on, so the purge needs no index of its own.
//
// Bounded and oldest-first for the same reason ArchiveDone is: a purge that
// cannot be interrupted is a purge that holds a connection for as long as the
// backlog takes.
func (s *RetentionStore) PruneArchive(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	if s == nil || s.db == nil || olderThan.IsZero() || limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("invalid outbox archive prune request")
	}
	result := s.db.WithContext(ctx).Exec(`
DELETE FROM event_delivery_archive AS archived
USING (
    SELECT event_id, consumer
    FROM event_delivery_archive
    WHERE completed_at < ?
    ORDER BY completed_at ASC
    LIMIT ?
) AS candidates
WHERE archived.event_id = candidates.event_id
  AND archived.consumer = candidates.consumer`, olderThan, limit)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

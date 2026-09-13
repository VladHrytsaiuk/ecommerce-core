package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PurgeTerminal deletes one bounded batch of terminal notification jobs.
//
// Only 'sent' and 'dead' qualify, and the exclusions matter more than the
// inclusions: 'pending' and 'sending' are work in progress, and 'failed' is the
// scheduled dispatcher's retryable state — it carries next_retry_at and will be
// picked up again — so deleting one silently drops a message that was still
// going to be sent.
//
// Two windows, because the two statuses mean opposite things. A 'sent' job is
// evidence a message went out and is only needed while someone might ask why;
// a 'dead' job is a message that never went out, which an operator may still
// need to see and act on, so it is kept far longer.
//
// notification_attempts rows go with the job through its ON DELETE CASCADE, so
// the delivery history of a purged job leaves with it rather than becoming
// orphaned.
func (r *Repository) PurgeTerminal(ctx context.Context, sentBefore, deadBefore time.Time, limit int) (int, error) {
	if r == nil || r.db == nil || sentBefore.IsZero() || deadBefore.IsZero() || limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("invalid notification retention request")
	}
	var purged int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// SKIP LOCKED so a purge never waits on the dispatcher holding a lease,
		// and never takes a row the dispatcher is working on.
		result := tx.Exec(`
WITH candidates AS (
    SELECT id FROM notification_jobs
    WHERE (status = 'sent' AND updated_at < ?)
       OR (status = 'dead' AND updated_at < ?)
    ORDER BY updated_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT ?
)
DELETE FROM notification_jobs AS job
USING candidates
WHERE job.id = candidates.id`, sentBefore.UTC(), deadBefore.UTC(), limit)
		if result.Error != nil {
			return result.Error
		}
		purged = result.RowsAffected
		return nil
	})
	return int(purged), err
}

// CountDead reports how many messages never went out.
//
// Retention eventually deletes these, so without a count the cleanup would
// quietly erase the evidence that a store has been failing to send mail. The
// number is sampled rather than incremented at the transition, so it survives a
// restart and reports what is actually in the table.
func (r *Repository) CountDead(ctx context.Context) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("notification retention store is not configured")
	}
	var dead int64
	if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM notification_jobs WHERE status = 'dead'`).Scan(&dead).Error; err != nil {
		return 0, err
	}
	return int(dead), nil
}

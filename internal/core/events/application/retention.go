package application

import (
	"context"
	"fmt"
	"time"
)

// RetentionStore ages terminal delivery metadata out of the database: first
// from the hot claim table into the archive, then out of the archive.
//
// domain_events itself is not pruned yet, and that is a pending decision rather
// than a rule that every event is kept forever. Only orders.paid.v1 and
// orders.refunded.v1 are kept permanently, as the source financial reports are
// rebuilt from; retention for the other topics is designed in
// docs/design/domain-events-retention.md.
type RetentionStore interface {
	ArchiveDone(context.Context, time.Time, int) (int, error)
	PruneArchive(context.Context, time.Time, int) (int, error)
}

type RetentionWorker struct {
	store            RetentionStore
	retention        time.Duration
	archiveRetention time.Duration
	batchSize        int
	logger           Logger
}

func NewRetentionWorker(store RetentionStore, retention, archiveRetention time.Duration, batchSize int, logger Logger) (*RetentionWorker, error) {
	if store == nil || retention <= 0 || batchSize < 1 || batchSize > 10000 {
		return nil, fmt.Errorf("invalid outbox retention worker configuration")
	}
	// The archive receives a delivery after retention has already elapsed, so
	// a shorter window would delete rows on arrival and keep no history at all.
	if archiveRetention < retention {
		return nil, fmt.Errorf("outbox archive retention (%s) must be at least the done retention (%s)", archiveRetention, retention)
	}
	return &RetentionWorker{store: store, retention: retention, archiveRetention: archiveRetention, batchSize: batchSize, logger: logger}, nil
}

func (w *RetentionWorker) ArchiveOnce(ctx context.Context) (int, error) {
	count, err := w.store.ArchiveDone(ctx, time.Now().UTC().Add(-w.retention), w.batchSize)
	if err == nil && count > 0 && w.logger != nil {
		w.logger.Infow("outbox terminal deliveries archived", "count", count)
	}
	return count, err
}

// PruneOnce empties one batch from the archive. Archiving alone only moved
// rows between two tables, so the database grew exactly as fast as it would
// have with no retention at all.
func (w *RetentionWorker) PruneOnce(ctx context.Context) (int, error) {
	count, err := w.store.PruneArchive(ctx, time.Now().UTC().Add(-w.archiveRetention), w.batchSize)
	if err == nil && count > 0 && w.logger != nil {
		w.logger.Infow("outbox archived deliveries pruned", "count", count)
	}
	return count, err
}

func (w *RetentionWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		// Archive first, then prune: a delivery that ages past both windows in
		// the same tick leaves the database in one pass instead of waiting a
		// further interval in the archive.
		if _, err := w.ArchiveOnce(ctx); err != nil && w.logger != nil {
			w.logger.Errorw("outbox retention failed", "error", err)
		}
		if _, err := w.PruneOnce(ctx); err != nil && w.logger != nil {
			w.logger.Errorw("outbox archive prune failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

package application

import (
	"context"
	"fmt"
	"time"
)

// RetentionStore archives terminal delivery metadata. Business events remain
// in domain_events because Audit and Reports use them as immutable history.
type RetentionStore interface {
	ArchiveDone(context.Context, time.Time, int) (int, error)
}

type RetentionWorker struct {
	store     RetentionStore
	retention time.Duration
	batchSize int
	logger    Logger
}

func NewRetentionWorker(store RetentionStore, retention time.Duration, batchSize int, logger Logger) (*RetentionWorker, error) {
	if store == nil || retention <= 0 || batchSize < 1 || batchSize > 10000 {
		return nil, fmt.Errorf("invalid outbox retention worker configuration")
	}
	return &RetentionWorker{store: store, retention: retention, batchSize: batchSize, logger: logger}, nil
}

func (w *RetentionWorker) ArchiveOnce(ctx context.Context) (int, error) {
	count, err := w.store.ArchiveDone(ctx, time.Now().UTC().Add(-w.retention), w.batchSize)
	if err == nil && count > 0 && w.logger != nil {
		w.logger.Infow("outbox terminal deliveries archived", "count", count)
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
		if _, err := w.ArchiveOnce(ctx); err != nil && w.logger != nil {
			w.logger.Errorw("outbox retention failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

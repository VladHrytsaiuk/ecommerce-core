package application

import (
	"context"
	"fmt"
	"time"

	media "github.com/VladHrytsaiuk/ecommerce-core/internal/media/domain"
)

const (
	OrphanCleanupPrefix   = "quarantine/"
	OrphanCleanupMaxAge   = 24 * time.Hour
	orphanCleanupBatchMax = 500
)

type CleanupLogger interface {
	Errorw(string, ...any)
}

// OrphanCleanupWorker reconciles the non-transactional object store with the
// authoritative media_assets table. It intentionally processes only old
// quarantine files, so an in-flight upload cannot be collected.
type OrphanCleanupWorker struct {
	repository media.AssetRepository
	store      media.ObjectStore
	maxAge     time.Duration
	now        func() time.Time
	logger     CleanupLogger
}

func NewOrphanCleanupWorker(repository media.AssetRepository, store media.ObjectStore, logger CleanupLogger) (*OrphanCleanupWorker, error) {
	if repository == nil || store == nil || logger == nil {
		return nil, fmt.Errorf("media orphan cleanup dependencies are required")
	}
	return &OrphanCleanupWorker{repository: repository, store: store, maxAge: OrphanCleanupMaxAge, now: time.Now, logger: logger}, nil
}

func (w *OrphanCleanupWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	w.cleanupAndLog(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanupAndLog(ctx)
		}
	}
}

func (w *OrphanCleanupWorker) cleanupAndLog(ctx context.Context) {
	// Bound external object-store work independently from the application
	// lifetime; a temporary storage outage must not pin a shutdown forever.
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := w.Cleanup(cleanupCtx); err != nil && ctx.Err() == nil {
		w.logger.Errorw("media orphan cleanup failed", "error", err)
	}
}

// Cleanup deletes unreferenced or failed assets only after the grace period.
// Database checks are batched to avoid an N+1 query for a large bucket.
func (w *OrphanCleanupWorker) Cleanup(ctx context.Context) error {
	if w == nil || w.repository == nil || w.store == nil {
		return fmt.Errorf("media orphan cleanup is not configured")
	}
	objects, err := w.store.List(ctx, OrphanCleanupPrefix)
	if err != nil {
		return err
	}
	cutoff := w.now().UTC().Add(-w.maxAge)
	candidates := make([]media.ListedObject, 0, len(objects))
	for _, object := range objects {
		if object.Key != "" && !object.LastModified.After(cutoff) {
			candidates = append(candidates, object)
		}
	}
	for start := 0; start < len(candidates); start += orphanCleanupBatchMax {
		end := start + orphanCleanupBatchMax
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]
		keys := make([]string, 0, len(batch))
		for _, object := range batch {
			keys = append(keys, object.Key)
		}
		referenced, err := w.repository.ReferencedObjectKeys(ctx, keys)
		if err != nil {
			return err
		}
		for _, object := range batch {
			if referenced[object.Key] {
				continue
			}
			if err := w.store.Delete(ctx, object.ObjectRef); err != nil {
				return fmt.Errorf("delete orphan %q: %w", object.Key, err)
			}
		}
	}
	return nil
}

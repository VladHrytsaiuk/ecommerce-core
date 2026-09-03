package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

const (
	VideoOrphanMaxAge        = 48 * time.Hour
	videoCleanupLeaseMaxAge  = 5 * time.Minute
	videoCleanupBatchSize    = 100
	videoCleanupTimeout      = 5 * time.Minute
	videoFinalizationTimeout = 3 * time.Second
)

type CleanupLogger interface {
	Errorw(string, ...any)
}

// OrphanCleanupWorker reconciles abandoned Direct Upload intents. It obtains a
// short database lease first, performs Cloudflare I/O after that transaction
// commits, then persists the final state in a separate short transaction.
// This deliberately avoids holding a PostgreSQL connection during network I/O.
type OrphanCleanupWorker struct {
	repository video.AssetRepository
	provider   video.VideoProvider
	logger     CleanupLogger
	now        func() time.Time
	maxAge     time.Duration
}

func NewOrphanCleanupWorker(repository video.AssetRepository, provider video.VideoProvider, logger CleanupLogger) (*OrphanCleanupWorker, error) {
	if repository == nil || provider == nil || logger == nil {
		return nil, fmt.Errorf("video orphan cleanup dependencies are required")
	}
	return &OrphanCleanupWorker{repository: repository, provider: provider, logger: logger, now: time.Now, maxAge: VideoOrphanMaxAge}, nil
}

func (w *OrphanCleanupWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
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
	cleanupCtx, cancel := context.WithTimeout(ctx, videoCleanupTimeout)
	defer cancel()
	if err := w.Cleanup(cleanupCtx); err != nil && ctx.Err() == nil {
		w.logger.Errorw("video orphan cleanup failed", "error", err)
	}
}

// Cleanup is exposed for deterministic tests and a manually invoked recovery
// job. It makes bounded progress and never exposes provider identifiers in
// logs.
func (w *OrphanCleanupWorker) Cleanup(ctx context.Context) error {
	if w == nil || w.repository == nil || w.provider == nil {
		return fmt.Errorf("video orphan cleanup is not configured")
	}
	now := w.now().UTC()
	assets, err := w.repository.ClaimStaleForCleanup(ctx, now.Add(-w.maxAge), now.Add(-videoCleanupLeaseMaxAge), videoCleanupBatchSize)
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if asset.ExternalID != "" {
			if err := w.provider.DeleteAsset(ctx, asset.ExternalID); err != nil {
				finalizationCtx, cancel := w.finalizationContext(ctx)
				restoreErr := w.repository.RestoreCleanup(finalizationCtx, asset.ID, cleanupRestoreStatus(asset))
				cancel()
				if restoreErr != nil {
					return fmt.Errorf("delete stale video asset: %w; restore cleanup lease: %v", err, restoreErr)
				}
				return fmt.Errorf("delete stale video asset: %w", err)
			}
		}
		finalizationCtx, cancel := w.finalizationContext(ctx)
		err := w.repository.MarkDeleted(finalizationCtx, asset.ID)
		cancel()
		if err != nil {
			return fmt.Errorf("mark stale video asset deleted: %w", err)
		}
	}
	return nil
}

func (w *OrphanCleanupWorker) finalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// A cancellation may arrive after Cloudflare has deleted the object. Keep a
	// tiny bounded window to persist the matching local state before shutdown
	// closes the DB pool.
	return context.WithTimeout(context.WithoutCancel(ctx), videoFinalizationTimeout)
}

func cleanupRestoreStatus(asset video.Asset) video.AssetStatus {
	if asset.Status == video.AssetDraft || asset.Status == video.AssetUploading || asset.Status == video.AssetProcessing {
		return asset.Status
	}
	if strings.TrimSpace(asset.ExternalID) == "" {
		return video.AssetDraft
	}
	return video.AssetUploading
}

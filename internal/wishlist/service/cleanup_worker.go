//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"go.uber.org/zap"
)

// WishlistCleanupWorker відповідає за періодичне очищення застарілих анонімних записів вішліста.
type WishlistCleanupWorker struct {
	repo   domain.WishlistRepository
	logger logger.Logger
	maxAge time.Duration // Максимальний вік анонімних записів (за замовчуванням 30 днів)
}

// NewWishlistCleanupWorker створює новий екземпляр воркера.
func NewWishlistCleanupWorker(repo domain.WishlistRepository, l logger.Logger) *WishlistCleanupWorker {
	return &WishlistCleanupWorker{
		repo:   repo,
		logger: l,
		maxAge: 14 * 24 * time.Hour, // 14 днів
	}
}

// Run executes the cleanup loop until ctx is cancelled.
func (w *WishlistCleanupWorker) Run(ctx context.Context, interval time.Duration) {
	w.logger.Info("Starting Wishlist Cleanup Worker", zap.Duration("interval", interval), zap.Duration("max_age", w.maxAge))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Перший запуск відразу при старті
	w.runCleanup(ctx)

	for {
		select {
		case <-ticker.C:
			w.runCleanup(ctx)
		case <-ctx.Done():
			w.logger.Info("Wishlist Cleanup Worker stopped")
			return
		}
	}
}

// runCleanup виконує один цикл очищення
func (w *WishlistCleanupWorker) runCleanup(ctx context.Context) {
	olderThan := time.Now().Add(-w.maxAge)

	if err := w.repo.DeleteExpiredAnonymous(ctx, olderThan); err != nil {
		w.logger.Error("Failed to cleanup expired anonymous wishlist items", zap.Error(err))
	}
}

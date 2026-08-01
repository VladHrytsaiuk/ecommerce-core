package service

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

// CartCleanupWorker відповідає за періодичне очищення застарілих анонімних кошиків.
type CartCleanupWorker struct {
	repo   domain.CartRepository
	logger logger.Logger
	maxAge time.Duration // Максимальний вік анонімних кошиків (за замовчуванням 14 днів)
}

// NewCartCleanupWorker створює новий екземпляр воркера.
func NewCartCleanupWorker(repo domain.CartRepository, l logger.Logger) *CartCleanupWorker {
	return &CartCleanupWorker{
		repo:   repo,
		logger: l,
		maxAge: 14 * 24 * time.Hour, // 14 днів
	}
}

// Run executes the cleanup loop until ctx is cancelled.
func (w *CartCleanupWorker) Run(ctx context.Context, interval time.Duration) {
	w.logger.Info("Starting Cart Cleanup Worker", zap.Duration("interval", interval), zap.Duration("max_age", w.maxAge))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Перший запуск відразу при старті
	w.runCleanup(ctx)

	for {
		select {
		case <-ticker.C:
			w.runCleanup(ctx)
		case <-ctx.Done():
			w.logger.Info("Cart Cleanup Worker stopped")
			return
		}
	}
}

// runCleanup виконує один цикл очищення
func (w *CartCleanupWorker) runCleanup(ctx context.Context) {
	olderThan := time.Now().Add(-w.maxAge)

	if err := w.repo.DeleteExpiredAnonymous(ctx, olderThan); err != nil {
		w.logger.Error("Failed to cleanup expired anonymous carts", zap.Error(err))
	}
}

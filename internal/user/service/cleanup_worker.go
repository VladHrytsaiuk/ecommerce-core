package service

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
)

// CleanupWorker відповідає за періодичне очищення застарілих даних із БД.
type CleanupWorker struct {
	sessionRepo    domain.SessionRepository
	verifyCodeRepo domain.VerifyCodeRepository
	logger         logger.Logger
}

// NewCleanupWorker створює новий екземпляр воркера.
func NewCleanupWorker(
	sRepo domain.SessionRepository,
	vRepo domain.VerifyCodeRepository,
	l logger.Logger,
) *CleanupWorker {
	return &CleanupWorker{
		sessionRepo:    sRepo,
		verifyCodeRepo: vRepo,
		logger:         l,
	}
}

// Start запускає цикл очищення у фоновій горутині.
// Очищення відбувається раз на заданий інтервал.
func (w *CleanupWorker) Start(ctx context.Context, interval time.Duration) {
	go w.Run(ctx, interval)
}

// Run executes the cleanup loop until ctx is cancelled.
func (w *CleanupWorker) Run(ctx context.Context, interval time.Duration) {
	w.logger.Info("Starting Cleanup Worker", zap.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Перший запуск відразу при старті
	w.runCleanup(ctx)

	for {
		select {
		case <-ticker.C:
			w.runCleanup(ctx)
		case <-ctx.Done():
			w.logger.Info("Cleanup Worker stopped")
			return
		}
	}
}

// runCleanup виконує один цикл очищення всіх підтримуваних таблиць.
func (w *CleanupWorker) runCleanup(ctx context.Context) {
	w.logger.Info("Running database cleanup cycle...")

	// 1. Очищення сесій
	if err := w.sessionRepo.DeleteExpired(ctx); err != nil {
		w.logger.Error("Failed to cleanup expired sessions", zap.Error(err))
	} else {
		w.logger.Info("Successfully cleaned up expired sessions")
	}

	// 2. Очищення кодів верифікації
	if err := w.verifyCodeRepo.DeleteExpired(ctx); err != nil {
		w.logger.Error("Failed to cleanup expired verify codes", zap.Error(err))
	} else {
		w.logger.Info("Successfully cleaned up expired verify codes")
	}
}

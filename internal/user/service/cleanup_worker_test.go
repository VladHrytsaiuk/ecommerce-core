package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCleanupWorker_RunCleanup(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	l := &noopLogger{}

	worker := NewCleanupWorker(sessionRepo, verifyCodeRepo, l)

	// Очікуємо, що обидва репозиторії отримають виклик DeleteExpired
	sessionRepo.On("DeleteExpired", mock.Anything).Return(nil).Once()
	verifyCodeRepo.On("DeleteExpired", mock.Anything).Return(nil).Once()

	worker.runCleanup(context.Background())

	sessionRepo.AssertExpectations(t)
	verifyCodeRepo.AssertExpectations(t)
}

func TestCleanupWorker_Start_Background(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	l := &noopLogger{}

	worker := NewCleanupWorker(sessionRepo, verifyCodeRepo, l)

	// Очікуємо декілька викликів (тікер + перший запуск)
	sessionRepo.On("DeleteExpired", mock.Anything).Return(nil)
	verifyCodeRepo.On("DeleteExpired", mock.Anything).Return(nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Запускаємо на дуже короткий інтервал (10мс)
	interval := 10 * time.Millisecond
	worker.Start(ctx, interval)

	// Даємо попрацювати трохи (50мс) — цього достатньо для декількох ітерацій
	time.Sleep(50 * time.Millisecond)
	cancel() // Останавливаем воркер

	// Перевіряємо, чи були виклики
	sessionRepo.AssertCalled(t, "DeleteExpired", mock.Anything)
	verifyCodeRepo.AssertCalled(t, "DeleteExpired", mock.Anything)
}

func TestCleanupWorker_RunCleanup_RepoErrors(t *testing.T) {
	// Воркер не повинен падати, якщо один з репозиторіїв повернув помилку
	sessionRepo := &MockSessionRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	l := &noopLogger{}

	worker := NewCleanupWorker(sessionRepo, verifyCodeRepo, l)

	sessionRepo.On("DeleteExpired", mock.Anything).Return(nil)
	verifyCodeRepo.On("DeleteExpired", mock.Anything).Return(context.DeadlineExceeded)

	// Виклик не повинен повернути помилку (він її лише логує)
	require.NotPanics(t, func() {
		worker.runCleanup(context.Background())
	})
}

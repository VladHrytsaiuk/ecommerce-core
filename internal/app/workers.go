package app

import (
	"context"
	"fmt"
	"time"
)

// Start begins background workers after the application dependency graph is
// fully assembled. It is safe to call more than once.
func (a *Application) Start(ctx context.Context) {
	a.workersMu.Lock()
	if a.workersStarted {
		a.workersMu.Unlock()
		return
	}

	workerCtx, cancel := context.WithCancel(ctx)
	a.workersStarted = true
	a.stopWorkers = cancel
	a.workersMu.Unlock()

	a.startWorker(func() { a.cleanupWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.wishlistCleanupWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.cartCleanupWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.SitemapWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.paymentWorker.Run(workerCtx, time.Minute) })
	a.startWorker(func() { a.trackingWorker.Run(workerCtx) })
}

// Stop requests worker cancellation and waits without a deadline. Prefer
// StopContext from server shutdown code so a stuck external dependency cannot
// block process termination indefinitely.
func (a *Application) Stop() {
	_ = a.StopContext(context.Background())
}

// StopContext requests worker cancellation and waits until all worker loops
// return or the supplied deadline expires.
func (a *Application) StopContext(ctx context.Context) error {
	a.workersMu.Lock()
	cancel := a.stopWorkers
	a.workersMu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()

	done := make(chan struct{})
	go func() {
		a.workersWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for application workers: %w", ctx.Err())
	}
}

func (a *Application) startWorker(run func()) {
	a.workersWG.Add(1)
	go func() {
		defer a.workersWG.Done()
		run()
	}()
}

package app

import (
	"context"
	"fmt"
	"time"
)

func (a *Application) Start(ctx context.Context) {
	a.workerMu.Lock()
	defer a.workerMu.Unlock()
	if a.workerCancel != nil || a.DeliveryDispatcher == nil || a.DeliveryTracker == nil || a.DeliveryCarriers == nil || a.DeliveryCarriers.Default() == nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	a.workerCancel = cancel
	a.workerWG.Add(1)
	go func() { defer a.workerWG.Done(); a.DeliveryDispatcher.Run(workerCtx, 5*time.Second) }()
	a.workerWG.Add(1)
	go func() { defer a.workerWG.Done(); a.DeliveryTracker.Run(workerCtx, 5*time.Minute) }()
}

func (a *Application) Stop() { _ = a.StopContext(context.Background()) }
func (a *Application) StopContext(ctx context.Context) error {
	a.workerMu.Lock()
	cancel := a.workerCancel
	a.workerCancel = nil
	a.workerMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	done := make(chan struct{})
	go func() { a.workerWG.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for workers: %w", ctx.Err())
	}
}

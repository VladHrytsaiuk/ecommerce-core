package app

import (
	"context"
	"fmt"
	"time"
)

func (a *Application) Start(ctx context.Context) {
	a.workerMu.Lock()
	defer a.workerMu.Unlock()
	if a.workerCancel != nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	a.workerCancel = cancel
	if a.DeliveryDispatcher != nil && a.DeliveryTracker != nil && a.DeliveryCarriers != nil && a.DeliveryCarriers.Default() != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.DeliveryDispatcher.Run(workerCtx, 5*time.Second) }()
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.DeliveryTracker.Run(workerCtx, 5*time.Minute) }()
	}
	if a.CheckoutRecovery != nil {
		a.workerWG.Add(1)
		go func() {
			defer a.workerWG.Done()
			a.CheckoutRecovery.Run(workerCtx, 5*time.Second, time.Minute, time.Minute)
		}()
	}
	if a.CheckoutExpiry != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.CheckoutExpiry.Run(workerCtx, time.Minute) }()
	}
	if a.InventoryCleanup != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.InventoryCleanup.Run(workerCtx, time.Minute) }()
	}
	if a.OutboxWorker != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.OutboxWorker.Run(workerCtx, 5*time.Second) }()
	}
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

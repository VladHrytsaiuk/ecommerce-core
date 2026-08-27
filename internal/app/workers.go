package app

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func (a *Application) Start(ctx context.Context) {
	a.workerMu.Lock()
	defer a.workerMu.Unlock()
	if a.workerCancel != nil {
		return
	}
	if a.Management != nil {
		if err := a.Management.Start(); err != nil {
			logger.Log.Errorw("management server failed to start", "error", err)
		} else {
			logger.Log.Infow("management server is listening", "address", a.Config.ManagementAddr)
		}
	}
	workerCtx, cancel := context.WithCancel(ctx)
	a.workerCancel = cancel
	if a.ReportsRebuilder != nil {
		a.ReportsRebuilder.BindLifecycle(workerCtx)
	}
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
	if a.AdminAuditOutboxWorker != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.AdminAuditOutboxWorker.Run(workerCtx, 5*time.Second) }()
	}
	if a.SearchOutboxWorker != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.SearchOutboxWorker.Run(workerCtx, 5*time.Second) }()
	}
	if a.MediaOutboxWorker != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.MediaOutboxWorker.Run(workerCtx, 5*time.Second) }()
	}
	if a.ReportsOutboxWorker != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.ReportsOutboxWorker.Run(workerCtx, 5*time.Second) }()
	}
	if a.OutboxRetention != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.OutboxRetention.Run(workerCtx, a.Config.OutboxRetentionInterval) }()
	}
	if a.MediaOrphanCleanup != nil {
		a.workerWG.Add(1)
		go func() { defer a.workerWG.Done(); a.MediaOrphanCleanup.Run(workerCtx, 24*time.Hour) }()
	}
}

func (a *Application) Stop() { _ = a.StopContext(context.Background()) }
func (a *Application) StopContext(ctx context.Context) error {
	a.workerMu.Lock()
	cancel := a.workerCancel
	a.workerCancel = nil
	closer := a.resourceCloser
	a.resourceCloser = nil
	managementServer := a.Management
	telemetryShutdown := a.telemetryShutdown
	a.telemetryShutdown = nil
	a.workerMu.Unlock()
	var shutdownErr error
	if managementServer != nil {
		if err := managementServer.Shutdown(ctx); err != nil {
			shutdownErr = fmt.Errorf("shutdown management server: %w", err)
		}
	}
	shutdownTelemetry := func() {
		if telemetryShutdown != nil {
			if err := telemetryShutdown(ctx); err != nil && shutdownErr == nil {
				shutdownErr = fmt.Errorf("shutdown telemetry: %w", err)
			}
		}
	}
	if cancel == nil {
		if closer != nil {
			if err := closer.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		shutdownTelemetry()
		return shutdownErr
	}
	cancel()
	if a.ReportsRebuilder != nil {
		if err := a.ReportsRebuilder.Wait(ctx); err != nil && shutdownErr == nil {
			shutdownErr = fmt.Errorf("wait for reports rebuild: %w", err)
		}
	}
	done := make(chan struct{})
	go func() { a.workerWG.Wait(); close(done) }()
	select {
	case <-done:
		if closer != nil {
			if err := closer.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		shutdownTelemetry()
		return shutdownErr
	case <-ctx.Done():
		if closer != nil {
			_ = closer.Close()
		}
		shutdownTelemetry()
		return fmt.Errorf("wait for workers: %w", ctx.Err())
	}
}

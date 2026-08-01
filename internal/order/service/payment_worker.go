package service

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// PaymentWorker фоновий воркер для перевірки тайм-аутів оплати
type PaymentWorker struct {
	orderSvc domain.OrderService
	logger   logger.Logger
}

// NewPaymentWorker створює новий PaymentWorker
func NewPaymentWorker(orderSvc domain.OrderService, l logger.Logger) *PaymentWorker {
	return &PaymentWorker{
		orderSvc: orderSvc,
		logger:   l,
	}
}

// Start запускає воркер
func (w *PaymentWorker) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		w.logger.Infow("Starting Payment Worker", "interval", interval)
		for {
			select {
			case <-ticker.C:
				if err := w.orderSvc.ProcessPaymentTimeouts(ctx); err != nil {
					w.logger.Errorw("Error in Payment Worker", "error", err)
				}
			case <-ctx.Done():
				w.logger.Info("Stopping Payment Worker")
				ticker.Stop()
				return
			}
		}
	}()
}

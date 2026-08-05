//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	shippingDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipping/domain"
)

// TrackingWorker фоновий воркер для оновлення статусів доставки
type TrackingWorker struct {
	orderRepo      domain.OrderRepository
	carrierService shippingDomain.CarrierService
	l              logger.Logger
	interval       time.Duration
}

// NewTrackingWorker створює новий інстанс воркера
func NewTrackingWorker(orderRepo domain.OrderRepository, carrierSvc shippingDomain.CarrierService, l logger.Logger, interval time.Duration) *TrackingWorker {
	if interval == 0 {
		interval = 30 * time.Minute // default
	}
	return &TrackingWorker{
		orderRepo:      orderRepo,
		carrierService: carrierSvc,
		l:              l,
		interval:       interval,
	}
}

// Run запускає воркер у безкінечному циклі
func (w *TrackingWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.l.Infow("Tracking worker started", "interval", w.interval)

	for {
		select {
		case <-ctx.Done():
			w.l.Info("Tracking worker stopped")
			return
		case <-ticker.C:
			w.processShippedOrders(ctx)
		}
	}
}

// processShippedOrders обробляє всі Shipped замовлення
func (w *TrackingWorker) processShippedOrders(ctx context.Context) {
	orders, err := w.orderRepo.GetShippedOrders(ctx)
	if err != nil {
		w.l.Errorw("TrackingWorker: failed to get shipped orders", "error", err)
		return
	}

	if len(orders) == 0 {
		return
	}

	w.l.Infow("TrackingWorker: found orders to track", "count", len(orders))

	for _, order := range orders {
		// Перевіряємо статус у перевізника
		result, err := w.carrierService.TrackShipment(ctx, order.TTNNumber, order.Phone)
		if err != nil {
			w.l.Errorw("TrackingWorker: failed to track shipment", "error", err, "order_number", order.OrderNumber, "ttn", order.TTNNumber)
			continue
		}

		// Якщо статус змінився або отримано нові дані (навіть якщо статус ще Shipped)
		if result.RawStatus != order.CarrierStatus {
			// Оновлюємо carrier_status та raw_response
			err = w.orderRepo.UpdateCarrierData(ctx, order.ID, result.RawStatus, &result.RawResponse)
			if err != nil {
				w.l.Errorw("TrackingWorker: failed to update carrier data", "error", err, "order_id", order.ID)
			}
		}

		// Якщо доставлено — оновлюємо статус замовлення та пишемо в історію
		if result.IsDelivered && order.StatusID == domain.StatusShipped {
			w.l.Infow("TrackingWorker: order delivered", "order_number", order.OrderNumber, "ttn", order.TTNNumber)

			err = w.orderRepo.WithTransaction(ctx, func(txCtx context.Context, txRepo domain.OrderRepository) error {
				// Блокуємо замовлення
				lockedOrder, err := txRepo.FindByIDForUpdate(txCtx, order.ID)
				if err != nil {
					return err
				}

				// Перевіряємо, чи воно досі Shipped
				if lockedOrder.StatusID != domain.StatusShipped {
					w.l.Infow("TrackingWorker: order status changed concurrently, skipping delivered update", "order_id", order.ID)
					return nil
				}

				// 1. Зміна статусу
				if err := txRepo.UpdateStatus(txCtx, order.ID, domain.StatusDelivered); err != nil {
					return err
				}
				// 2. Історія
				fromStatus := domain.StatusShipped
				if err := txRepo.CreateStatusHistory(txCtx, &domain.OrderStatusHistory{
					OrderID:      order.ID,
					FromStatusID: &fromStatus,
					ToStatusID:   domain.StatusDelivered,
					Source:       domain.SourceNovaPoshta,
					Comment:      ptr("Автоматичне оновлення за трекінгом НП"),
				}); err != nil {
					return err
				}
				return nil
			})

			if err != nil {
				w.l.Errorw("TrackingWorker: failed to update order to delivered", "error", err, "order_id", order.ID)
			}
		}
	}
}

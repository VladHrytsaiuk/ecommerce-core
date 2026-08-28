package application

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

type Tracker struct {
	store    domain.TrackingStore
	carriers *Registry
	orders   domain.OrderTransitioner
}

func NewTracker(store domain.TrackingStore, carriers *Registry, orderTransitioners ...domain.OrderTransitioner) *Tracker {
	var orders domain.OrderTransitioner
	if len(orderTransitioners) > 0 {
		orders = orderTransitioners[0]
	}
	return &Tracker{store: store, carriers: carriers, orders: orders}
}
func (t *Tracker) ReconcileOnce(ctx context.Context) error {
	if t.store == nil || t.carriers == nil {
		return nil
	}
	deliveries, err := t.store.ListActive(ctx, 100)
	if err != nil {
		return err
	}
	for _, delivery := range deliveries {
		carrier, ok := t.carriers.Get(delivery.Provider)
		if !ok {
			continue
		}
		result, err := carrier.Track(ctx, domain.TrackingRequest{TrackingNumber: delivery.TrackingNumber, RecipientPhone: delivery.RecipientPhone})
		if err != nil {
			continue
		}
		if err := t.store.UpdateStatusAndTransition(ctx, delivery, result, t.orders); err != nil {
			return err
		}
	}
	return nil
}
func (t *Tracker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = t.ReconcileOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

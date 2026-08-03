package application

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

type Tracker struct {
	store    domain.TrackingStore
	carriers *Registry
}

func NewTracker(store domain.TrackingStore, carriers *Registry) *Tracker {
	return &Tracker{store: store, carriers: carriers}
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
		if err := t.store.UpdateStatus(ctx, delivery.ID, result); err != nil {
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

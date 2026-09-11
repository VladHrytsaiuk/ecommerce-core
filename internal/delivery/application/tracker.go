package application

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

type Tracker struct {
	store    domain.TrackingStore
	carriers *Registry
	orders   domain.OrderTransitioner
	logger   worker.Logger
}

// WithLogger surfaces a failing reconciliation. A tracker that cannot reach
// its store otherwise looks like one with no active deliveries.
func (t *Tracker) WithLogger(logger worker.Logger) *Tracker {
	if t != nil && logger != nil {
		t.logger = logger
	}
	return t
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

// Run reconciles a page of active deliveries each tick. It does not drain:
// ReconcileOnce already walks up to a hundred per pass, and tracking is a poll
// of an external carrier rather than a queue to empty.
func (t *Tracker) Run(ctx context.Context, interval time.Duration) {
	worker.Loop(ctx, interval, 5*time.Minute, t.logger, "delivery tracker", t.ReconcileOnce)
}

package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

// Dispatcher delivers durable order events outside the transaction which
// created them. It never performs a remote call while holding a DB lock.
type Dispatcher struct {
	outbox     domain.OutboxStore
	exporter   domain.OrderExporter
	retryDelay time.Duration
	lease      time.Duration
}

func NewDispatcher(outbox domain.OutboxStore, exporter domain.OrderExporter, retryDelay, lease time.Duration) *Dispatcher {
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	if lease <= 0 {
		lease = time.Minute
	}
	return &Dispatcher{outbox: outbox, exporter: exporter, retryDelay: retryDelay, lease: lease}
}

func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	if d.outbox == nil || d.exporter == nil {
		return fmt.Errorf("sync dispatcher is not configured")
	}
	now := time.Now().UTC()
	event, err := d.outbox.Claim(ctx, now, d.lease)
	if err != nil || event == nil {
		return err
	}
	if event.Topic != domain.TopicOrderCreated {
		return d.outbox.Retry(context.WithoutCancel(ctx), event.ID, fmt.Errorf("unsupported sync topic %q", event.Topic), now.Add(d.retryDelay))
	}
	if err := d.exporter.ExportOrder(ctx, *event); err != nil {
		return d.outbox.Retry(context.WithoutCancel(ctx), event.ID, err, now.Add(d.retryDelay))
	}
	return d.outbox.Complete(context.WithoutCancel(ctx), event.ID, time.Now().UTC())
}

func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = d.DispatchOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

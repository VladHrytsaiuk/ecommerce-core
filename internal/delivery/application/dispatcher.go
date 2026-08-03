package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

type Dispatcher struct {
	jobs       domain.JobStore
	carriers   *Registry
	retryDelay time.Duration
}

func NewDispatcher(jobs domain.JobStore, carriers *Registry, retryDelay time.Duration) *Dispatcher {
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	return &Dispatcher{jobs: jobs, carriers: carriers, retryDelay: retryDelay}
}
func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	if d.jobs == nil || d.carriers == nil {
		return fmt.Errorf("delivery dispatcher is not configured")
	}
	job, err := d.jobs.Claim(ctx, time.Now().UTC())
	if err != nil || job == nil {
		return err
	}
	carrier, ok := d.carriers.Get(job.Provider)
	if !ok {
		return d.jobs.Fail(context.WithoutCancel(ctx), job.ID, fmt.Errorf("delivery carrier %q is not enabled", job.Provider))
	}
	result, err := carrier.CreateShipment(ctx, domain.CreateShipmentRequest{OrderID: job.OrderID, IdempotencyKey: job.IdempotencyKey.String(), Destination: job.Destination, Items: job.Items, DeclaredValue: job.DeclaredValue})
	if err != nil {
		return d.jobs.Retry(context.WithoutCancel(ctx), job.ID, err, time.Now().UTC().Add(d.retryDelay))
	}
	return d.jobs.Complete(context.WithoutCancel(ctx), job.ID, result)
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

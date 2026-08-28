package application

import (
	"context"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
)

type Dispatcher struct {
	jobs        domain.JobStore
	carriers    *Registry
	retryDelay  time.Duration
	maxAttempts int
}

// MaxAttempts bounds ambiguous carrier retries. A dead job is deliberately
// retained for manual reconciliation instead of silently creating TTN copies.
const MaxAttempts = 5

func NewDispatcher(jobs domain.JobStore, carriers *Registry, retryDelay time.Duration) *Dispatcher {
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	return &Dispatcher{jobs: jobs, carriers: carriers, retryDelay: retryDelay, maxAttempts: MaxAttempts}
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
	// Reconcile before issuing a non-idempotent provider create. Nova Poshta
	// stores this stable UUID in InfoRegClientBarcodes; a timeout can therefore
	// be resolved without creating a second waybill.
	if finder, ok := carrier.(domain.ShipmentFinder); ok {
		result, findErr := finder.FindShipment(ctx, job.IdempotencyKey.String())
		if findErr != nil {
			return d.retryOrDead(ctx, job, findErr)
		}
		if result != nil {
			return d.jobs.Complete(context.WithoutCancel(ctx), job.ID, *result)
		}
	}
	result, err := carrier.CreateShipment(ctx, domain.CreateShipmentRequest{OrderID: job.OrderID, IdempotencyKey: job.IdempotencyKey.String(), Destination: job.Destination, Items: job.Items, DeclaredValue: job.DeclaredValue})
	if err != nil {
		return d.retryOrDead(ctx, job, err)
	}
	return d.jobs.Complete(context.WithoutCancel(ctx), job.ID, result)
}

func (d *Dispatcher) retryOrDead(ctx context.Context, job *domain.DispatchJob, cause error) error {
	finalizeCtx := context.WithoutCancel(ctx)
	if job.Attempts >= d.maxAttempts {
		return d.jobs.Dead(finalizeCtx, job.ID, cause)
	}
	return d.jobs.Retry(finalizeCtx, job.ID, cause, time.Now().UTC().Add(d.retryDelay))
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

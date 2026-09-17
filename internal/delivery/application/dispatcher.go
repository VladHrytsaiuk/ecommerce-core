package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/worker"
)

type Dispatcher struct {
	jobs        domain.JobStore
	carriers    *Registry
	retryDelay  time.Duration
	maxAttempts int
	logger      worker.Logger
}

// WithLogger makes a failing pass visible. Without it the dispatcher is silent
// by construction: an unreachable database looks exactly like an empty queue.
func (d *Dispatcher) WithLogger(logger worker.Logger) *Dispatcher {
	if d != nil && logger != nil {
		d.logger = logger
	}
	return d
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

// DispatchOnce settles at most one job. It reports whether it found one, so a
// drain can tell an empty queue from a completed unit of work.
func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	_, err := d.dispatchOnce(ctx)
	return err
}

func (d *Dispatcher) dispatchOnce(ctx context.Context) (bool, error) {
	if d.jobs == nil || d.carriers == nil {
		return false, fmt.Errorf("delivery dispatcher is not configured")
	}
	job, err := d.jobs.Claim(ctx, time.Now().UTC())
	if err != nil || job == nil {
		return false, err
	}
	carrier, ok := d.carriers.Get(job.Provider)
	if !ok {
		return true, d.jobs.Fail(context.WithoutCancel(ctx), *job, fmt.Errorf("delivery carrier %q is not enabled", job.Provider))
	}
	// Reconcile before issuing a non-idempotent provider create. Nova Poshta
	// stores this stable UUID in InfoRegClientBarcodes; a timeout can therefore
	// be resolved without creating a second waybill.
	finder, canReconcile := carrier.(domain.ShipmentFinder)
	if canReconcile {
		result, findErr := finder.FindShipment(ctx, job.IdempotencyKey.String())
		if findErr != nil {
			return true, d.retryOrDead(ctx, job, findErr)
		}
		if result != nil {
			return true, d.jobs.Complete(context.WithoutCancel(ctx), *job, *result)
		}
	}
	// Re-dispatching after an ambiguous failure is the one case where a
	// second waybill gets printed. A timeout is indistinguishable from a
	// success whose response was lost, so unless the carrier can be asked what
	// happened, the job is parked for a human rather than tried again — the
	// same reasoning MaxAttempts already applies, one attempt earlier.
	//
	// A failure the adapter marked ErrShipmentNotSent is exempt: it promises
	// nothing was created, so there is nothing to duplicate.
	if !canReconcile && job.Attempts > 1 && !job.LastFailureWasDefinite {
		return true, d.jobs.Dead(context.WithoutCancel(ctx), *job,
			fmt.Errorf("delivery carrier %q cannot confirm whether attempt %d created a shipment; reconcile manually before re-dispatching", job.Provider, job.Attempts-1))
	}
	result, err := carrier.CreateShipment(ctx, domain.CreateShipmentRequest{OrderID: job.OrderID, IdempotencyKey: job.IdempotencyKey.String(), Destination: job.Destination, Items: job.Items, DeclaredValue: job.DeclaredValue})
	if err != nil {
		return true, d.retryOrDead(ctx, job, err)
	}
	return true, d.jobs.Complete(context.WithoutCancel(ctx), *job, result)
}

func (d *Dispatcher) retryOrDead(ctx context.Context, job *domain.DispatchJob, cause error) error {
	finalizeCtx := context.WithoutCancel(ctx)
	if job.Attempts >= d.maxAttempts {
		return d.jobs.Dead(finalizeCtx, *job, cause)
	}
	// Record whether this failure is known to have created nothing, so the next
	// claim can tell a safe retry from a possible duplicate.
	return d.jobs.Retry(finalizeCtx, *job, cause, time.Now().UTC().Add(d.retryDelay), errors.Is(cause, domain.ErrShipmentNotSent))
}

// Run drains the job queue each tick rather than settling one shipment per
// interval. On the default five-second tick that was twelve dispatches an
// hour: a backlog from any outage took days to clear.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	worker.LoopDraining(ctx, interval, 5*time.Second, d.logger, "delivery dispatcher", d.dispatchOnce)
}

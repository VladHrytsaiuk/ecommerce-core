package application

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

const (
	// maxDrainPerTick bounds one drain pass so a dispatcher with a large
	// backlog cannot hold its database connection or delay shutdown.
	maxDrainPerTick = 256
	// maxRetryBackoffShift caps the retry schedule at roughly sixty-four times
	// the base delay, so the attempts span long enough to outlast an ERP outage.
	maxRetryBackoffShift = 6
)

// errEventRescheduled reports an export failure that was durably recorded
// against the event. The queue itself is healthy, which is what lets a drain
// continue; a store failure returns an unwrapped error and stops the pass.
var errEventRescheduled = errors.New("sync outbox event rescheduled")

// Dispatcher delivers durable order events outside the transaction which
// created them. It never performs a remote call while holding a DB lock.
type Dispatcher struct {
	outbox      domain.OutboxStore
	exporter    domain.OrderExporter
	retryDelay  time.Duration
	lease       time.Duration
	maxAttempts int
}

func NewDispatcher(outbox domain.OutboxStore, exporter domain.OrderExporter, retryDelay, lease time.Duration, maxAttempts int) *Dispatcher {
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	if lease <= 0 {
		lease = time.Minute
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	return &Dispatcher{outbox: outbox, exporter: exporter, retryDelay: retryDelay, lease: lease, maxAttempts: maxAttempts}
}

// DispatchOnce exports at most one event. Run drains instead; this stays the
// single-step entry point for tests and operational tooling.
func (d *Dispatcher) DispatchOnce(ctx context.Context) error {
	_, err := d.dispatchOnce(ctx)
	return err
}

// dispatchOnce reports whether an event was claimed, so drain can tell an
// exhausted queue from a processed one and stop without waiting for a tick.
func (d *Dispatcher) dispatchOnce(ctx context.Context) (bool, error) {
	if d.outbox == nil || d.exporter == nil {
		return false, fmt.Errorf("sync dispatcher is not configured")
	}
	now := time.Now().UTC()
	event, err := d.outbox.Claim(ctx, now, d.lease)
	if err != nil || event == nil {
		return false, err
	}
	if event.Topic != domain.TopicOrderCreated {
		return true, d.fail(context.WithoutCancel(ctx), event, fmt.Errorf("unsupported sync topic %q", event.Topic), now)
	}
	if err := d.exporter.ExportOrder(ctx, *event); err != nil {
		return true, d.fail(context.WithoutCancel(ctx), event, err, now)
	}
	if err := d.outbox.Complete(context.WithoutCancel(ctx), event.ID, event.LockedAt, time.Now().UTC()); err != nil {
		// Another dispatcher took the event over after this one overran its
		// lease. That dispatcher owns the outcome, so this is a normal race.
		if errors.Is(err, domain.ErrLeaseLost) {
			return true, nil
		}
		return true, err
	}
	return true, nil
}

// drain exports ready events until the queue is exhausted or the batch ceiling
// is reached. Returning to the ticker after a single event capped the
// dispatcher at one export per interval, so a backlog could never be cleared.
func (d *Dispatcher) drain(ctx context.Context) error {
	for range maxDrainPerTick {
		if ctx.Err() != nil {
			return nil
		}
		claimed, err := d.dispatchOnce(ctx)
		if err != nil {
			// A rescheduled event is durably recorded and will not be re-claimed
			// now, so the queue is healthy and the batch must not wait for a tick.
			if errors.Is(err, errEventRescheduled) {
				continue
			}
			return err
		}
		if !claimed {
			return nil
		}
	}
	return nil
}

func (d *Dispatcher) fail(ctx context.Context, event *domain.OutboxEvent, cause error, now time.Time) error {
	var err error
	if event.Attempts >= d.maxAttempts {
		err = d.outbox.DeadLetter(ctx, event.ID, cause, event.LockedAt, now)
	} else {
		err = d.outbox.Retry(ctx, event.ID, cause, event.LockedAt, now.Add(d.retryAfter(event.Attempts)))
	}
	if err != nil {
		if errors.Is(err, domain.ErrLeaseLost) {
			return fmt.Errorf("%w: lease lost", errEventRescheduled)
		}
		return err
	}
	return fmt.Errorf("%w: %v", errEventRescheduled, cause)
}

// retryAfter backs off exponentially with jitter. A flat delay exhausted every
// attempt inside a few minutes, so an ERP outage lasting longer buried each
// affected export in the dead-letter queue even though a later retry would
// have succeeded. The jitter stops a batch that failed together from returning
// as one synchronized herd.
func (d *Dispatcher) retryAfter(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	backoff := d.retryDelay << min(attempts-1, maxRetryBackoffShift)
	return backoff + time.Duration(rand.Int64N(int64(backoff/2)))
}

func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = d.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

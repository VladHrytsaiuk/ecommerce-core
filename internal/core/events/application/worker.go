// Package application contains event delivery orchestration without concrete
// database or logging dependencies.
package application

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/sanitize"
)

const (
	DefaultMaxAttempts  = 10
	finalizationTimeout = 3 * time.Second
	// maxDrainPerTick bounds one drain pass so a consumer with a large backlog
	// cannot hold its database connection or delay shutdown indefinitely.
	maxDrainPerTick = 256
	// baseRetryDelay and maxRetryBackoffShift cap the retry schedule at about
	// an hour, so ten attempts span long enough to outlast a provider outage.
	baseRetryDelay       = time.Minute
	maxRetryBackoffShift = 6
)

// errDeliveryRescheduled reports a handler failure that was durably recorded
// against the delivery. The queue itself is healthy, which is what lets drain
// continue; a store failure returns an unwrapped error and stops the pass.
var errDeliveryRescheduled = errors.New("event delivery rescheduled")

type Logger interface {
	Infow(string, ...interface{})
	Errorw(string, ...interface{})
}

// Span is the minimal lifecycle contract the worker needs from tracing. It
// keeps OpenTelemetry and all other platform concerns outside the application
// layer while allowing an adapter to preserve trace continuity per delivery.
type Span interface {
	End()
}

// Tracer resumes the trace context stored with a durable delivery and starts
// its consumer span. Platform implementations can use OpenTelemetry; tests
// and deployments without tracing use the no-op default.
type Tracer interface {
	ContinueDelivery(context.Context, events.Delivery) (context.Context, Span)
}

// Consumer handles one event topic after its delivery lease is claimed. It is
// invoked outside the claim transaction and must be idempotent.
type Consumer interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
}

// OutboxWorker currently acknowledges claimed events after structured logging.
// Notifications will replace this acknowledgement with its durable job
// creation in the next increment; no external I/O occurs here.
type OutboxWorker struct {
	store       events.DeliveryStore
	consumer    string
	lease       time.Duration
	maxAttempts int
	logger      Logger
	tracer      Tracer
	handlers    map[string][]Consumer
}

func NewOutboxWorker(store events.DeliveryStore, consumer string, lease time.Duration, logger Logger, handlers ...Consumer) *OutboxWorker {
	if lease <= 0 {
		lease = time.Minute
	}
	registered := make(map[string][]Consumer, len(handlers))
	for _, handler := range handlers {
		if handler != nil && handler.Topic() != "" {
			registered[handler.Topic()] = append(registered[handler.Topic()], handler)
		}
	}
	return &OutboxWorker{store: store, consumer: consumer, lease: lease, maxAttempts: DefaultMaxAttempts, logger: logger, tracer: noopTracer{}, handlers: registered}
}

// WithTracer injects the platform tracing adapter at the composition root.
// Nil intentionally retains no-op behaviour for unit tests and minimal
// deployments.
func (w *OutboxWorker) WithTracer(tracer Tracer) *OutboxWorker {
	if tracer != nil {
		w.tracer = tracer
	}
	return w
}

func (w *OutboxWorker) WithMaxAttempts(maxAttempts int) *OutboxWorker {
	if maxAttempts > 0 {
		w.maxAttempts = maxAttempts
	}
	return w
}

// DispatchOnce claims and processes at most one delivery. Run drains instead;
// this stays the single-step entry point for tests and operational tooling.
func (w *OutboxWorker) DispatchOnce(ctx context.Context) error {
	_, err := w.dispatchOnce(ctx)
	return err
}

// dispatchOnce reports whether a delivery was claimed so that drain can tell
// an exhausted queue from a processed one and stop without waiting for a tick.
func (w *OutboxWorker) dispatchOnce(ctx context.Context) (bool, error) {
	if w.store == nil || w.consumer == "" {
		return false, fmt.Errorf("event outbox worker is not configured")
	}
	event, err := w.store.Claim(ctx, w.consumer, time.Now().UTC(), w.lease)
	if err != nil || event == nil {
		return false, err
	}
	ctx, span := w.tracer.ContinueDelivery(ctx, *event)
	defer span.End()
	if w.logger != nil {
		w.logger.Infow("event outbox delivery claimed", "event_id", event.EventID, "topic", event.Topic, "consumer", event.Consumer, "attempt", event.Attempts)
	}
	handlers, registered := w.handlers[event.Topic]
	if !registered {
		// This consumer was given a delivery for a topic it cannot handle,
		// which is a routing mistake rather than a processed event. Falling
		// through to Complete would mark it done and erase an event nobody
		// ever looked at — silently, and without leaving a failed row for
		// anyone to notice. Failing it keeps the event in domain_events and
		// makes the mismatch visible in event_deliveries.
		return true, w.failDelivery(ctx, event, fmt.Errorf("consumer %q has no handler for topic %q", w.consumer, event.Topic))
	}
	for _, handler := range handlers {
		if err := invokeSafely(ctx, handler, *event); err != nil {
			return true, w.failDelivery(ctx, event, err)
		}
	}
	finalizationCtx, cancel := finalizationContext(ctx)
	defer cancel()
	if err := w.store.Complete(finalizationCtx, event.EventID, w.consumer, event.LockedAt, time.Now().UTC()); err != nil {
		// Losing the lease means another worker took the delivery over after
		// this one overran. That worker owns the outcome, so this is a normal
		// race, not a fault: reporting it as an error would fill the log with
		// alerts naming the worker that was still doing legitimate work.
		if errors.Is(err, events.ErrLeaseLost) {
			if w.logger != nil {
				w.logger.Infow("event outbox delivery lease lost before completion", "event_id", event.EventID, "topic", event.Topic, "consumer", w.consumer, "attempt", event.Attempts)
			}
			return true, nil
		}
		if w.logger != nil {
			w.logger.Errorw("event outbox delivery completion failed", "event_id", event.EventID, "consumer", w.consumer, "error_code", sanitize.ErrorCode(err))
		}
		return true, err
	}
	if w.logger != nil {
		w.logger.Infow("event outbox delivery completed", "event_id", event.EventID, "topic", event.Topic, "consumer", event.Consumer)
	}
	return true, nil
}

// drain processes ready deliveries until the queue is exhausted or the batch
// ceiling is reached. Returning to the ticker after a single delivery capped a
// consumer at one event per interval, so a backlog could never be worked off.
// The ceiling keeps one busy consumer from holding its database connection
// indefinitely and bounds the work done between shutdown checks.
func (w *OutboxWorker) drain(ctx context.Context) error {
	for range maxDrainPerTick {
		if ctx.Err() != nil {
			return nil
		}
		claimed, err := w.dispatchOnce(ctx)
		if err != nil {
			// A rescheduled delivery is already durably recorded and will not
			// be re-claimed now, so the queue is healthy and the rest of the
			// batch must not wait for the next tick.
			if errors.Is(err, errDeliveryRescheduled) {
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

type noopTracer struct{}
type noopSpan struct{}

func (noopTracer) ContinueDelivery(ctx context.Context, _ events.Delivery) (context.Context, Span) {
	return ctx, noopSpan{}
}

func (noopSpan) End() {}

func (w *OutboxWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.drain(ctx); err != nil && ctx.Err() == nil && w.logger != nil {
			w.logger.Errorw("event outbox worker failed", "consumer", w.consumer, "error_code", sanitize.ErrorCode(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *OutboxWorker) failDelivery(ctx context.Context, event *events.Delivery, cause error) error {
	code := sanitize.ErrorCode(cause)
	sanitizedCause := errors.New(code)
	finalizationCtx, cancel := finalizationContext(ctx)
	defer cancel()
	var err error
	dead := event.Attempts >= w.maxAttempts
	now := time.Now().UTC()
	if dead {
		err = w.store.Dead(finalizationCtx, event.EventID, w.consumer, sanitizedCause, event.LockedAt, now)
	} else {
		err = w.store.Fail(finalizationCtx, event.EventID, w.consumer, sanitizedCause, event.LockedAt, now.Add(retryDelay(event.Attempts)))
	}
	if err != nil {
		// Another worker already owns this delivery, so it will record the
		// outcome. Nothing is lost and the drain can continue.
		if errors.Is(err, events.ErrLeaseLost) {
			if w.logger != nil {
				w.logger.Infow("event outbox delivery lease lost before failure was recorded", "event_id", event.EventID, "topic", event.Topic, "consumer", w.consumer, "attempt", event.Attempts)
			}
			return fmt.Errorf("%w: lease lost", errDeliveryRescheduled)
		}
		// The failure could not be recorded, so the delivery keeps its claim
		// until the lease expires. This is a store problem, not a handler one.
		return fmt.Errorf("record event delivery failure")
	}
	if w.logger != nil {
		status := "failed"
		if dead {
			status = "dead"
		}
		w.logger.Errorw("event outbox delivery failed", "event_id", event.EventID, "topic", event.Topic, "consumer", event.Consumer, "status", status, "error_code", code)
	}
	return fmt.Errorf("%w: %s", errDeliveryRescheduled, code)
}

// finalizationContext allows a claimed delivery to be acknowledged even when
// the parent was cancelled, but bounds the final database call so shutdown can
// never wait indefinitely for an unavailable PostgreSQL connection.
func finalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finalizationTimeout)
}

// retryDelay backs off exponentially with jitter. A flat one-minute delay
// exhausted all ten attempts inside ten minutes, so a provider outage lasting
// longer than that buried every affected event in the dead-letter queue even
// though a later retry would have succeeded. The jitter keeps a batch of
// events that failed together from returning as a synchronized thundering herd.
func retryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	shift := min(attempts-1, maxRetryBackoffShift)
	backoff := baseRetryDelay << shift
	return backoff + time.Duration(rand.Int64N(int64(backoff/2)))
}

func invokeSafely(ctx context.Context, handler Consumer, event events.Delivery) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("handler_panic")
		}
	}()
	return handler.Handle(ctx, event)
}

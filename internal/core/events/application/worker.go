// Package application contains event delivery orchestration without concrete
// database or logging dependencies.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/sanitize"
)

const DefaultMaxAttempts = 10

type Logger interface {
	Infow(string, ...interface{})
	Errorw(string, ...interface{})
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
	handlers    map[string]Consumer
}

func NewOutboxWorker(store events.DeliveryStore, consumer string, lease time.Duration, logger Logger, handlers ...Consumer) *OutboxWorker {
	if lease <= 0 {
		lease = time.Minute
	}
	registered := make(map[string]Consumer, len(handlers))
	for _, handler := range handlers {
		if handler != nil && handler.Topic() != "" {
			registered[handler.Topic()] = handler
		}
	}
	return &OutboxWorker{store: store, consumer: consumer, lease: lease, maxAttempts: DefaultMaxAttempts, logger: logger, handlers: registered}
}

func (w *OutboxWorker) WithMaxAttempts(maxAttempts int) *OutboxWorker {
	if maxAttempts > 0 {
		w.maxAttempts = maxAttempts
	}
	return w
}

func (w *OutboxWorker) DispatchOnce(ctx context.Context) error {
	if w.store == nil || w.consumer == "" {
		return fmt.Errorf("event outbox worker is not configured")
	}
	event, err := w.store.Claim(ctx, w.consumer, time.Now().UTC(), w.lease)
	if err != nil || event == nil {
		return err
	}
	if w.logger != nil {
		w.logger.Infow("event outbox delivery claimed", "event_id", event.EventID, "topic", event.Topic, "consumer", event.Consumer, "attempt", event.Attempts)
	}
	if handler := w.handlers[event.Topic]; handler != nil {
		if err := invokeSafely(ctx, handler, *event); err != nil {
			return w.failDelivery(ctx, event, err)
		}
	}
	if err := w.store.Complete(context.WithoutCancel(ctx), event.EventID, w.consumer, time.Now().UTC()); err != nil {
		if w.logger != nil {
			w.logger.Errorw("event outbox delivery completion failed", "event_id", event.EventID, "consumer", w.consumer, "error_code", sanitize.ErrorCode(err))
		}
		return err
	}
	if w.logger != nil {
		w.logger.Infow("event outbox delivery completed", "event_id", event.EventID, "topic", event.Topic, "consumer", event.Consumer)
	}
	return nil
}

func (w *OutboxWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.DispatchOnce(ctx); err != nil && w.logger != nil {
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
	var err error
	if event.Attempts >= w.maxAttempts {
		err = w.store.Dead(context.WithoutCancel(ctx), event.EventID, w.consumer, sanitizedCause, time.Now().UTC())
	} else {
		err = w.store.Fail(context.WithoutCancel(ctx), event.EventID, w.consumer, sanitizedCause, time.Now().UTC().Add(time.Minute))
	}
	if err != nil {
		return fmt.Errorf("record event delivery failure")
	}
	if w.logger != nil {
		status := "failed"
		if event.Attempts >= w.maxAttempts {
			status = "dead"
		}
		w.logger.Errorw("event outbox delivery failed", "event_id", event.EventID, "topic", event.Topic, "consumer", event.Consumer, "status", status, "error_code", code)
	}
	return fmt.Errorf("event delivery failed: %s", code)
}

func invokeSafely(ctx context.Context, handler Consumer, event events.Delivery) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("handler_panic")
		}
	}()
	return handler.Handle(ctx, event)
}

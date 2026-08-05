package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/sync/domain"
)

func TestDispatcherCompletesExportedOrder(t *testing.T) {
	event := &domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated, AggregateID: uuid.New(), IdempotencyKey: uuid.New()}
	store := &fakeOutbox{event: event}
	dispatcher := NewDispatcher(store, &fakeExporter{}, time.Minute, time.Minute, 10)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if store.completed != event.ID || store.retried != uuid.Nil {
		t.Fatalf("outbox result = completed:%s retried:%s", store.completed, store.retried)
	}
}

func TestDispatcherRetriesFailedExport(t *testing.T) {
	event := &domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated}
	store := &fakeOutbox{event: event}
	dispatcher := NewDispatcher(store, &fakeExporter{err: errors.New("ERP unavailable")}, time.Minute, time.Minute, 10)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if store.retried != event.ID || store.completed != uuid.Nil || store.retryAt.Before(time.Now().UTC().Add(59*time.Second)) {
		t.Fatalf("outbox result = completed:%s retried:%s retryAt:%s", store.completed, store.retried, store.retryAt)
	}
}

func TestDispatcherRejectsUnknownTopicWithoutCallingExporter(t *testing.T) {
	event := &domain.OutboxEvent{ID: uuid.New(), Topic: "catalog.deleted"}
	store := &fakeOutbox{event: event}
	exporter := &fakeExporter{}
	dispatcher := NewDispatcher(store, exporter, time.Minute, time.Minute, 10)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if exporter.called || store.retried != event.ID {
		t.Fatalf("unknown topic exporterCalled:%t retried:%s", exporter.called, store.retried)
	}
}

func TestDispatcherDeadLettersAfterMaximumAttempts(t *testing.T) {
	event := &domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated, Attempts: 3}
	store := &fakeOutbox{event: event}
	dispatcher := NewDispatcher(store, &fakeExporter{err: errors.New("permanent ERP rejection")}, time.Minute, time.Minute, 3)
	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.dead != event.ID || store.retried != uuid.Nil {
		t.Fatalf("dead:%s retry:%s", store.dead, store.retried)
	}
}

type fakeOutbox struct {
	event     *domain.OutboxEvent
	completed uuid.UUID
	retried   uuid.UUID
	retryAt   time.Time
	dead      uuid.UUID
}

func (f *fakeOutbox) Claim(context.Context, time.Time, time.Duration) (*domain.OutboxEvent, error) {
	return f.event, nil
}
func (f *fakeOutbox) Complete(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.completed = id
	return nil
}
func (f *fakeOutbox) Retry(_ context.Context, id uuid.UUID, _ error, at time.Time) error {
	f.retried, f.retryAt = id, at
	return nil
}
func (f *fakeOutbox) DeadLetter(_ context.Context, id uuid.UUID, _ error, _ time.Time) error {
	f.dead = id
	return nil
}

type fakeExporter struct {
	err    error
	called bool
}

func (f *fakeExporter) ExportOrder(context.Context, domain.OutboxEvent) error {
	f.called = true
	return f.err
}

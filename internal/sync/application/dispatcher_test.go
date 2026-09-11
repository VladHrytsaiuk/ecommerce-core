package application

import (
	"context"
	"errors"
	"fmt"
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

	// A recorded failure is still a failed export, so it reaches the caller;
	// the sentinel is what tells a drain the queue itself is healthy.
	if err := dispatcher.DispatchOnce(context.Background()); !errors.Is(err, errEventRescheduled) {
		t.Fatalf("DispatchOnce() error = %v, want a rescheduled event", err)
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

	if err := dispatcher.DispatchOnce(context.Background()); !errors.Is(err, errEventRescheduled) {
		t.Fatalf("DispatchOnce() error = %v, want a rescheduled event", err)
	}
	if exporter.called || store.retried != event.ID {
		t.Fatalf("unknown topic exporterCalled:%t retried:%s", exporter.called, store.retried)
	}
}

func TestDispatcherDeadLettersAfterMaximumAttempts(t *testing.T) {
	event := &domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated, Attempts: 3}
	store := &fakeOutbox{event: event}
	dispatcher := NewDispatcher(store, &fakeExporter{err: errors.New("permanent ERP rejection")}, time.Minute, time.Minute, 3)
	if err := dispatcher.DispatchOnce(context.Background()); !errors.Is(err, errEventRescheduled) {
		t.Fatalf("DispatchOnce() error = %v, want a rescheduled event", err)
	}
	if store.dead != event.ID || store.retried != uuid.Nil {
		t.Fatalf("dead:%s retry:%s", store.dead, store.retried)
	}
}

type fakeOutbox struct {
	event            *domain.OutboxEvent
	completed        uuid.UUID
	completeLockedAt time.Time
	completeErr      error
	retried          uuid.UUID
	retryAt          time.Time
	dead             uuid.UUID
}

func (f *fakeOutbox) Claim(context.Context, time.Time, time.Duration) (*domain.OutboxEvent, error) {
	return f.event, nil
}
func (f *fakeOutbox) Complete(_ context.Context, id uuid.UUID, lockedAt, _ time.Time) error {
	f.completed, f.completeLockedAt = id, lockedAt
	return f.completeErr
}
func (f *fakeOutbox) Retry(_ context.Context, id uuid.UUID, _ error, _, at time.Time) error {
	f.retried, f.retryAt = id, at
	return nil
}
func (f *fakeOutbox) DeadLetter(_ context.Context, id uuid.UUID, _ error, _, _ time.Time) error {
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

func TestDispatcherPassesClaimLeaseToCompletion(t *testing.T) {
	lockedAt := time.Date(2026, 3, 4, 10, 30, 0, 0, time.UTC)
	event := &domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated, LockedAt: lockedAt}
	store := &fakeOutbox{event: event}

	if err := NewDispatcher(store, &fakeExporter{}, time.Minute, time.Minute, 10).DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	// Without the token the store can only match on status, which a re-claim
	// leaves unchanged, so a slow dispatcher would finalize someone else's work.
	if !store.completeLockedAt.Equal(lockedAt) {
		t.Fatalf("completion lease = %s, want the lease returned by Claim (%s)", store.completeLockedAt, lockedAt)
	}
}

func TestDispatcherTreatsLostLeaseAsNormalOutcome(t *testing.T) {
	store := &fakeOutbox{
		event:       &domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated},
		completeErr: fmt.Errorf("%w: re-claimed", domain.ErrLeaseLost),
	}

	// The event now belongs to whichever dispatcher re-claimed it, so raising
	// an error here would alert on the one that behaved correctly.
	if err := NewDispatcher(store, &fakeExporter{}, time.Minute, time.Minute, 10).DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v, want a lost lease handled as a normal outcome", err)
	}
}

func TestDispatcherDrainsTheWholeBacklogInOnePass(t *testing.T) {
	store := &queueingOutbox{pending: syncBacklog(12)}
	exporter := &fakeExporter{}

	if err := NewDispatcher(store, exporter, time.Minute, time.Minute, 10).drain(context.Background()); err != nil {
		t.Fatalf("drain() error = %v", err)
	}
	// One event per tick capped the dispatcher at twelve exports per minute,
	// so a backlog grew faster than it could ever be worked off.
	if len(store.completed) != 12 {
		t.Fatalf("completed = %d, want the whole backlog in a single pass", len(store.completed))
	}
}

func TestDispatcherDrainContinuesPastARescheduledEvent(t *testing.T) {
	store := &queueingOutbox{pending: syncBacklog(5)}
	exporter := &flakyExporter{failures: 1}

	if err := NewDispatcher(store, exporter, time.Minute, time.Minute, 10).drain(context.Background()); err != nil {
		t.Fatalf("drain() error = %v", err)
	}
	if len(store.retried) != 1 || len(store.completed) != 4 {
		t.Fatalf("retried/completed = %d/%d, want 1/4: one bad export must not strand the batch",
			len(store.retried), len(store.completed))
	}
}

func TestDispatcherRetryBacksOffExponentially(t *testing.T) {
	dispatcher := NewDispatcher(&fakeOutbox{}, &fakeExporter{}, time.Minute, time.Minute, 10)
	// A flat delay burned every attempt within minutes, so an ERP outage
	// lasting longer sent each affected export to the dead-letter queue.
	for _, testCase := range []struct {
		attempts       int
		atLeast, below time.Duration
	}{
		{attempts: 1, atLeast: time.Minute, below: 90 * time.Second},
		{attempts: 4, atLeast: 8 * time.Minute, below: 12 * time.Minute},
		{attempts: 20, atLeast: 64 * time.Minute, below: 96 * time.Minute},
	} {
		delay := dispatcher.retryAfter(testCase.attempts)
		if delay < testCase.atLeast || delay >= testCase.below {
			t.Fatalf("retryAfter(%d) = %s, want within [%s, %s)", testCase.attempts, delay, testCase.atLeast, testCase.below)
		}
	}
}

func syncBacklog(count int) []domain.OutboxEvent {
	events := make([]domain.OutboxEvent, 0, count)
	for range count {
		events = append(events, domain.OutboxEvent{ID: uuid.New(), Topic: domain.TopicOrderCreated})
	}
	return events
}

// queueingOutbox serves a finite backlog so a whole drain pass can be observed,
// unlike fakeOutbox which replays one event forever.
type queueingOutbox struct {
	pending   []domain.OutboxEvent
	completed []uuid.UUID
	retried   []uuid.UUID
}

func (q *queueingOutbox) Claim(context.Context, time.Time, time.Duration) (*domain.OutboxEvent, error) {
	if len(q.pending) == 0 {
		return nil, nil
	}
	event := q.pending[0]
	q.pending = q.pending[1:]
	return &event, nil
}
func (q *queueingOutbox) Complete(_ context.Context, id uuid.UUID, _, _ time.Time) error {
	q.completed = append(q.completed, id)
	return nil
}
func (q *queueingOutbox) Retry(_ context.Context, id uuid.UUID, _ error, _, _ time.Time) error {
	q.retried = append(q.retried, id)
	return nil
}
func (q *queueingOutbox) DeadLetter(_ context.Context, id uuid.UUID, _ error, _, _ time.Time) error {
	q.retried = append(q.retried, id)
	return nil
}

// flakyExporter fails a fixed number of leading exports, modelling an ERP
// outage that clears while the batch is still draining.
type flakyExporter struct {
	failures int
	calls    int
}

func (f *flakyExporter) ExportOrder(context.Context, domain.OutboxEvent) error {
	f.calls++
	if f.calls <= f.failures {
		return errors.New("ERP temporarily unavailable")
	}
	return nil
}

var _ domain.OutboxStore = (*queueingOutbox)(nil)

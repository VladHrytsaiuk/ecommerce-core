package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

func TestOutboxWorkerClaimsAndCompletesOneDelivery(t *testing.T) {
	eventID := uuid.New()
	store := &fakeDeliveryStore{delivery: &events.Delivery{EventID: eventID, Topic: events.TopicOrderPaid, Consumer: events.ConsumerNotifications}}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil)

	if err := worker.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if store.claimConsumer != events.ConsumerNotifications || store.completeEventID != eventID || store.completeConsumer != events.ConsumerNotifications {
		t.Fatalf("claim/complete = %q / %s:%q", store.claimConsumer, store.completeEventID, store.completeConsumer)
	}
}

func TestOutboxWorkerDoesNotCompleteWhenClaimFails(t *testing.T) {
	store := &fakeDeliveryStore{claimErr: errors.New("database unavailable")}
	err := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil).DispatchOnce(context.Background())
	if err == nil || store.completeEventID != uuid.Nil {
		t.Fatalf("DispatchOnce() = %v, completed = %s", err, store.completeEventID)
	}
}

func TestOutboxWorkerRecoversHandlerPanicAndFailsDelivery(t *testing.T) {
	eventID := uuid.New()
	store := &fakeDeliveryStore{delivery: &events.Delivery{EventID: eventID, Topic: events.TopicOrderPaid, Consumer: events.ConsumerNotifications, Attempts: 1}}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, panicConsumer{}).WithMaxAttempts(5)
	if err := worker.DispatchOnce(context.Background()); err == nil {
		t.Fatal("DispatchOnce() error = nil, want panic converted to failure")
	}
	if store.failedEventID != eventID || store.deadEventID != uuid.Nil || store.failCause == nil {
		t.Fatalf("failure/dead state = %s/%s/%v", store.failedEventID, store.deadEventID, store.failCause)
	}
}

func TestOutboxWorkerMovesFinalFailureToDeadLetter(t *testing.T) {
	eventID := uuid.New()
	store := &fakeDeliveryStore{delivery: &events.Delivery{EventID: eventID, Topic: events.TopicOrderPaid, Consumer: events.ConsumerNotifications, Attempts: 5}}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, errorConsumer{}).WithMaxAttempts(5)
	if err := worker.DispatchOnce(context.Background()); err == nil {
		t.Fatal("DispatchOnce() error = nil, want failure")
	}
	if store.deadEventID != eventID || store.failedEventID != uuid.Nil {
		t.Fatalf("failure/dead state = %s/%s", store.failedEventID, store.deadEventID)
	}
}

func TestOutboxWorkerBoundsFinalizationAfterParentCancellation(t *testing.T) {
	eventID := uuid.New()
	store := &fakeDeliveryStore{delivery: &events.Delivery{EventID: eventID, Topic: events.TopicOrderPaid, Consumer: events.ConsumerNotifications}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil).DispatchOnce(ctx); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if !store.completeHasDeadline || store.completeContextErr != nil {
		t.Fatalf("completion context deadline=%t err=%v; want active bounded context", store.completeHasDeadline, store.completeContextErr)
	}
}

func TestOutboxWorkerDispatchesEveryHandlerForATopic(t *testing.T) {
	t.Parallel()
	store := &fakeDeliveryStore{delivery: &events.Delivery{EventID: uuid.New(), Topic: events.TopicOrderPaid, Consumer: events.ConsumerReportsProjection}}
	first, second := &countingConsumer{}, &countingConsumer{}
	worker := NewOutboxWorker(store, events.ConsumerReportsProjection, time.Minute, nil, first, second)
	if err := worker.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if first.calls != 1 || second.calls != 1 || store.completeEventID == uuid.Nil {
		t.Fatalf("handler calls = %d/%d, completed = %s", first.calls, second.calls, store.completeEventID)
	}
}

func TestOutboxWorkerUsesInjectedTracerForDelivery(t *testing.T) {
	eventID := uuid.New()
	store := &fakeDeliveryStore{delivery: &events.Delivery{
		EventID: eventID, Topic: events.TopicOrderPaid, Consumer: events.ConsumerNotifications,
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}}
	tracer := &recordingTracer{}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil).WithTracer(tracer)

	if err := worker.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if tracer.delivery.EventID != eventID || !tracer.span.ended {
		t.Fatalf("tracer delivery/end = %s/%t, want %s/true", tracer.delivery.EventID, tracer.span.ended, eventID)
	}
}

type fakeDeliveryStore struct {
	delivery            *events.Delivery
	claimErr            error
	claimConsumer       string
	completeEventID     uuid.UUID
	completeConsumer    string
	completeHasDeadline bool
	completeContextErr  error
	failedEventID       uuid.UUID
	deadEventID         uuid.UUID
	failCause           error
}

func (s *fakeDeliveryStore) Claim(_ context.Context, consumer string, _ time.Time, _ time.Duration) (*events.Delivery, error) {
	s.claimConsumer = consumer
	return s.delivery, s.claimErr
}

func (s *fakeDeliveryStore) Complete(ctx context.Context, eventID uuid.UUID, consumer string, _ time.Time) error {
	s.completeEventID, s.completeConsumer = eventID, consumer
	_, s.completeHasDeadline = ctx.Deadline()
	s.completeContextErr = ctx.Err()
	return nil
}

func (s *fakeDeliveryStore) Fail(_ context.Context, eventID uuid.UUID, _ string, cause error, _ time.Time) error {
	s.failedEventID, s.failCause = eventID, cause
	return nil
}
func (s *fakeDeliveryStore) Dead(_ context.Context, eventID uuid.UUID, _ string, cause error, _ time.Time) error {
	s.deadEventID, s.failCause = eventID, cause
	return nil
}

type panicConsumer struct{}

func (panicConsumer) Topic() string                                 { return events.TopicOrderPaid }
func (panicConsumer) Handle(context.Context, events.Delivery) error { panic("unexpected template nil") }

type errorConsumer struct{}

func (errorConsumer) Topic() string { return events.TopicOrderPaid }
func (errorConsumer) Handle(context.Context, events.Delivery) error {
	return errors.New("smtp rejected buyer@example.com")
}

type countingConsumer struct{ calls int }

func (*countingConsumer) Topic() string { return events.TopicOrderPaid }
func (c *countingConsumer) Handle(context.Context, events.Delivery) error {
	c.calls++
	return nil
}

type recordingTracer struct {
	delivery events.Delivery
	span     recordingSpan
}

func (t *recordingTracer) ContinueDelivery(ctx context.Context, delivery events.Delivery) (context.Context, Span) {
	t.delivery = delivery
	return ctx, &t.span
}

type recordingSpan struct{ ended bool }

func (s *recordingSpan) End() { s.ended = true }

var _ events.DeliveryStore = (*fakeDeliveryStore)(nil)

func TestOutboxWorkerDrainsWholeBacklogInOnePass(t *testing.T) {
	store := &queuedDeliveryStore{pending: backlog(12)}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, &countingConsumer{})

	if err := worker.drain(context.Background()); err != nil {
		t.Fatalf("drain() error = %v", err)
	}
	// One delivery per tick capped a consumer at twelve events per minute, so
	// a backlog grew faster than it could ever be worked off.
	if len(store.completed) != 12 {
		t.Fatalf("completed = %d, want the whole backlog of 12 in a single pass", len(store.completed))
	}
}

func TestOutboxWorkerDrainContinuesPastRescheduledDelivery(t *testing.T) {
	store := &queuedDeliveryStore{pending: backlog(5)}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, &flakyConsumer{failures: 1}).WithMaxAttempts(10)

	if err := worker.drain(context.Background()); err != nil {
		t.Fatalf("drain() error = %v", err)
	}
	if len(store.rescheduled) != 1 || len(store.completed) != 4 {
		t.Fatalf("rescheduled/completed = %d/%d, want 1/4: a rescheduled delivery must not strand its batch",
			len(store.rescheduled), len(store.completed))
	}
}

func TestOutboxWorkerDrainStopsWhenStoreFails(t *testing.T) {
	store := &queuedDeliveryStore{pending: backlog(5), claimErrAt: 3}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, &countingConsumer{})

	if err := worker.drain(context.Background()); err == nil {
		t.Fatal("drain() error = nil, want the store failure surfaced to the caller")
	}
	if len(store.completed) != 2 {
		t.Fatalf("completed = %d, want only the two deliveries claimed before the store failed", len(store.completed))
	}
}

func TestOutboxWorkerDrainStopsAtBatchCeiling(t *testing.T) {
	store := &queuedDeliveryStore{pending: backlog(maxDrainPerTick + 10)}
	worker := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, &countingConsumer{})

	if err := worker.drain(context.Background()); err != nil {
		t.Fatalf("drain() error = %v", err)
	}
	if len(store.completed) != maxDrainPerTick || len(store.pending) != 10 {
		t.Fatalf("completed/pending = %d/%d, want %d/10", len(store.completed), len(store.pending), maxDrainPerTick)
	}
}

func TestOutboxWorkerDrainStopsOnCancellation(t *testing.T) {
	store := &queuedDeliveryStore{pending: backlog(5)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewOutboxWorker(store, events.ConsumerNotifications, time.Minute, nil, &countingConsumer{}).drain(ctx); err != nil {
		t.Fatalf("drain() error = %v", err)
	}
	if len(store.completed) != 0 {
		t.Fatalf("completed = %d, want no delivery claimed after cancellation", len(store.completed))
	}
}

func backlog(count int) []events.Delivery {
	deliveries := make([]events.Delivery, 0, count)
	for range count {
		deliveries = append(deliveries, events.Delivery{
			EventID: uuid.New(), Topic: events.TopicOrderPaid, Consumer: events.ConsumerNotifications,
		})
	}
	return deliveries
}

// queuedDeliveryStore serves a finite backlog so a whole drain pass can be
// observed, unlike fakeDeliveryStore which replays one delivery forever.
type queuedDeliveryStore struct {
	pending     []events.Delivery
	claims      int
	claimErrAt  int // 1-based claim that fails; zero never fails.
	completed   []uuid.UUID
	rescheduled []uuid.UUID
}

func (s *queuedDeliveryStore) Claim(context.Context, string, time.Time, time.Duration) (*events.Delivery, error) {
	s.claims++
	if s.claimErrAt != 0 && s.claims == s.claimErrAt {
		return nil, errors.New("database unavailable")
	}
	if len(s.pending) == 0 {
		return nil, nil
	}
	delivery := s.pending[0]
	s.pending = s.pending[1:]
	return &delivery, nil
}

func (s *queuedDeliveryStore) Complete(_ context.Context, eventID uuid.UUID, _ string, _ time.Time) error {
	s.completed = append(s.completed, eventID)
	return nil
}

func (s *queuedDeliveryStore) Fail(_ context.Context, eventID uuid.UUID, _ string, _ error, _ time.Time) error {
	s.rescheduled = append(s.rescheduled, eventID)
	return nil
}

func (s *queuedDeliveryStore) Dead(_ context.Context, eventID uuid.UUID, _ string, _ error, _ time.Time) error {
	s.rescheduled = append(s.rescheduled, eventID)
	return nil
}

// flakyConsumer fails a fixed number of leading deliveries, modelling a
// provider outage that clears while the batch is still being drained.
type flakyConsumer struct {
	failures int
	calls    int
}

func (*flakyConsumer) Topic() string { return events.TopicOrderPaid }

func (c *flakyConsumer) Handle(context.Context, events.Delivery) error {
	c.calls++
	if c.calls <= c.failures {
		return errors.New("smtp temporarily unavailable")
	}
	return nil
}

var _ events.DeliveryStore = (*queuedDeliveryStore)(nil)

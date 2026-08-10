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

type fakeDeliveryStore struct {
	delivery         *events.Delivery
	claimErr         error
	claimConsumer    string
	completeEventID  uuid.UUID
	completeConsumer string
	failedEventID    uuid.UUID
	deadEventID      uuid.UUID
	failCause        error
}

func (s *fakeDeliveryStore) Claim(_ context.Context, consumer string, _ time.Time, _ time.Duration) (*events.Delivery, error) {
	s.claimConsumer = consumer
	return s.delivery, s.claimErr
}

func (s *fakeDeliveryStore) Complete(_ context.Context, eventID uuid.UUID, consumer string, _ time.Time) error {
	s.completeEventID, s.completeConsumer = eventID, consumer
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

var _ events.DeliveryStore = (*fakeDeliveryStore)(nil)

package projectors

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

func TestFunnelProjectorCountsLifecycleEventsOnce(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{}
	projector, err := NewFunnelProjector(repository, fakeTransactions{}, events.TopicCartCreated, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	cartID, eventID := uuid.New(), uuid.New()
	event, err := events.NewCartCreatedEvent(cartID, time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	delivery := events.Delivery{EventID: eventID, AggregateID: cartID, Topic: events.TopicCartCreated, Payload: payload, OccurredAt: time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC)}
	if err := projector.Handle(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	if err := projector.Handle(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	if len(repository.funnels) != 1 || repository.funnels[0].CartsCreated != 1 || repository.funnels[0].CheckoutsStarted != 0 {
		t.Fatalf("funnel = %+v", repository.funnels)
	}
}

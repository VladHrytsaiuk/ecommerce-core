package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

// These payloads are the wire contract between modules: they are written to
// domain_events, read back by a worker in another module, and decoded against a
// struct that module owns. A field dropped or renamed here does not fail a
// build — it fails at runtime, once, in a consumer, and the delivery retries ten
// times before dying quietly. So the shape is worth stating.

func TestEveryEventPayloadCarriesItsVersion(t *testing.T) {
	// Consumers reject a version they do not know rather than guessing at a
	// payload. Version 1 is what every one of them checks for.
	for name, event := range allSampleEvents(t) {
		t.Run(name, func(t *testing.T) {
			raw, err := event.MarshalPayload()
			if err != nil {
				t.Fatalf("MarshalPayload() error = %v", err)
			}
			var envelope struct {
				Version int `json:"version"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("payload is not an object: %v", err)
			}
			if envelope.Version != 1 {
				t.Fatalf("version = %d, want 1", envelope.Version)
			}
		})
	}
}

func TestAnEventIdentifiesItsAggregateAndTopic(t *testing.T) {
	// The topic decides which consumers are handed the delivery, and the
	// aggregate id is what an operator greps for when one goes wrong.
	for name, event := range allSampleEvents(t) {
		t.Run(name, func(t *testing.T) {
			if event.Topic() == "" {
				t.Fatal("Topic() is empty; the router would have nothing to match")
			}
			if event.AggregateType() == "" {
				t.Fatal("AggregateType() is empty")
			}
			if event.AggregateID() == uuid.Nil {
				t.Fatal("AggregateID() is nil; the event names nothing")
			}
			if event.IdempotencyKey() == uuid.Nil {
				t.Fatal("IdempotencyKey() is nil; redelivery would duplicate")
			}
			if event.OccurredAt().IsZero() {
				t.Fatal("OccurredAt() is zero")
			}
			if event.OccurredAt().Location() != time.UTC {
				t.Fatalf("OccurredAt() is in %v, want UTC", event.OccurredAt().Location())
			}
		})
	}
}

func TestTheOrderPaidPayloadCarriesTheMoneyTheReceiptNeeds(t *testing.T) {
	// The notifications handler renders a receipt from exactly these fields,
	// and money crosses as minor units plus a currency, never as a float.
	orderID := uuid.New()
	event, err := NewOrderPaidEvent(orderID, "  ORDER-3F7A9B21C4D0  ", mustMoney(t, 129900, "EUR"), time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Version     int       `json:"version"`
		OrderID     uuid.UUID `json:"order_id"`
		OrderNumber string    `json:"order_number"`
		TotalAmount int64     `json:"total_amount"`
		Currency    string    `json:"currency"`
		PaidAt      time.Time `json:"paid_at"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OrderID != orderID || payload.OrderNumber != "ORDER-3F7A9B21C4D0" {
		t.Fatalf("payload = %+v, want the trimmed number and this order", payload)
	}
	if payload.TotalAmount != 129900 || payload.Currency != "EUR" {
		t.Fatalf("total = %d %s, want minor units and a currency", payload.TotalAmount, payload.Currency)
	}
	if !payload.PaidAt.Equal(event.PaidAt) {
		t.Fatalf("paid_at = %v, want %v", payload.PaidAt, event.PaidAt)
	}
}

func TestAnEventIsRefusedRatherThanBuiltIncomplete(t *testing.T) {
	// An event that names no aggregate cannot be routed, deduplicated or
	// traced, and it would be discovered only by the consumer that received it.
	valid := mustMoney(t, 1000, "EUR")
	var zero money.Money

	if _, err := NewOrderPaidEvent(uuid.Nil, "ORDER-1", valid, time.Now()); err == nil {
		t.Fatal("NewOrderPaidEvent() accepted a nil order")
	}
	if _, err := NewOrderPaidEvent(uuid.New(), "   ", valid, time.Now()); err == nil {
		t.Fatal("NewOrderPaidEvent() accepted a blank order number")
	}
	if _, err := NewOrderPaidEvent(uuid.New(), "ORDER-1", zero, time.Now()); err == nil {
		t.Fatal("NewOrderPaidEvent() accepted money that does not validate")
	}
	if _, err := NewOrderRefundedEvent(uuid.Nil, valid, time.Now()); err == nil {
		t.Fatal("NewOrderRefundedEvent() accepted a nil order")
	}
	if _, err := NewCartCreatedEvent(uuid.Nil, time.Now()); err == nil {
		t.Fatal("NewCartCreatedEvent() accepted a nil cart")
	}
	if _, err := NewCartUpdatedEvent(uuid.Nil, nil, time.Now()); err == nil {
		t.Fatal("NewCartUpdatedEvent() accepted a nil cart")
	}
	if _, err := NewCheckoutStartedEvent(uuid.Nil, uuid.New(), time.Now()); err == nil {
		t.Fatal("NewCheckoutStartedEvent() accepted a nil order")
	}
	if _, err := NewCheckoutEmailCapturedEvent(uuid.Nil, time.Now()); err == nil {
		t.Fatal("NewCheckoutEmailCapturedEvent() accepted a nil cart")
	}
}

func TestAnAbsentTimestampBecomesNowRatherThanZero(t *testing.T) {
	// A zero OccurredAt would sort the event to the beginning of time in every
	// timeline that reads it.
	before := time.Now().UTC().Add(-time.Second)
	event, err := NewOrderPaidEvent(uuid.New(), "ORDER-1", mustMoney(t, 1000, "EUR"), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if event.PaidAt.Before(before) {
		t.Fatalf("PaidAt = %v, want roughly now", event.PaidAt)
	}
}

func TestATimestampIsNormalisedToUTC(t *testing.T) {
	// Events are compared and ordered across modules; a local zone here would
	// make that ordering depend on where the process runs.
	zone := time.FixedZone("UTC+5", 5*60*60)
	local := time.Date(2026, time.September, 12, 15, 0, 0, 0, zone)
	event, err := NewOrderRefundedEvent(uuid.New(), mustMoney(t, 1000, "EUR"), local)
	if err != nil {
		t.Fatal(err)
	}
	if event.At.Location() != time.UTC || !event.At.Equal(local) {
		t.Fatalf("At = %v in %v, want the same instant in UTC", event.At, event.At.Location())
	}
}

func TestTopicsAreDistinct(t *testing.T) {
	// Two events sharing a topic would be handed to each other's consumer,
	// which decodes a payload it cannot read.
	seen := map[string]string{}
	for name, event := range allSampleEvents(t) {
		if previous, clash := seen[event.Topic()]; clash {
			t.Fatalf("%s and %s both publish %q", previous, name, event.Topic())
		}
		seen[event.Topic()] = name
	}
}

func allSampleEvents(t *testing.T) map[string]DomainEvent {
	t.Helper()
	now := time.Now().UTC()
	cartID, orderID, customerID := uuid.New(), uuid.New(), uuid.New()
	total := mustMoney(t, 2500, "EUR")

	paid, err := NewOrderPaidEvent(orderID, "ORDER-1", total, now)
	if err != nil {
		t.Fatal(err)
	}
	refunded, err := NewOrderRefundedEvent(orderID, total, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := NewCartCreatedEvent(cartID, now)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := NewCartUpdatedEvent(cartID, &customerID, now)
	if err != nil {
		t.Fatal(err)
	}
	started, err := NewCheckoutStartedEvent(orderID, cartID, now)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := NewCheckoutEmailCapturedEvent(cartID, now)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]DomainEvent{
		"order paid":              paid,
		"order refunded":          refunded,
		"cart created":            created,
		"cart updated":            updated,
		"checkout started":        started,
		"checkout email captured": captured,
	}
}

func mustMoney(t *testing.T, amount int64, currency string) money.Money {
	t.Helper()
	value, err := money.NewMoney(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

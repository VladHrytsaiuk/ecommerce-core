package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOrderStatusChangedEventIsVersionedAndIdempotent(t *testing.T) {
	orderID, transitionID := uuid.New(), uuid.New()
	event, err := NewOrderStatusChangedEvent(orderID, "processing", "shipped", StatusActorAdmin, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC), transitionID)
	if err != nil {
		t.Fatalf("NewOrderStatusChangedEvent() error = %v", err)
	}
	if event.Topic() != TopicOrderStatusChanged || event.AggregateID() != orderID || event.IdempotencyKey() != transitionID {
		t.Fatalf("event contract = %#v", event)
	}
	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["version"] != float64(1) || decoded["from_status"] != "processing" || decoded["to_status"] != "shipped" || decoded["actor_type"] != "admin" {
		t.Fatalf("payload = %s", payload)
	}
	if _, exists := decoded["transition_id"]; exists {
		t.Fatalf("payload leaks internal idempotency key: %s", payload)
	}
}

func TestOrderStatusChangedEventRejectsInvalidFinancialAuditInput(t *testing.T) {
	_, err := NewOrderStatusChangedEvent(uuid.New(), "paid", "refunded", StatusActorType("untrusted"), time.Now(), uuid.New())
	if err == nil {
		t.Fatal("NewOrderStatusChangedEvent() accepted invalid actor type")
	}
}

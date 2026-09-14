package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTheErasureTopicSaysWhatItRecords(t *testing.T) {
	// The event is written after the erasure has happened. It used to be
	// called privacy.erasure_requested.v1.
	if TopicErasureCompleted != "privacy.erasure_completed.v1" {
		t.Fatalf("topic = %q", TopicErasureCompleted)
	}
}

func TestTheErasureRecordCarriesNoIdentifierOfThePerson(t *testing.T) {
	// The one record of an erasure used to keep the raw customer_id the
	// erasure removed, in an append-only table.
	requestID := uuid.New()
	event, err := NewErasureCompletedEvent(requestID, "erasure-policy-2026-09", time.Unix(1_800_000_000, 0))
	if err != nil {
		t.Fatalf("NewErasureCompletedEvent() error = %v", err)
	}
	raw, err := event.MarshalPayload()
	if err != nil {
		t.Fatalf("MarshalPayload() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	for _, forbidden := range []string{"customer_id", "user_id", "email"} {
		if _, present := payload[forbidden]; present {
			t.Fatalf("payload carries %q: %s", forbidden, raw)
		}
	}
	// Exactly the audit fact the design names: request, time, outcome, policy.
	want := map[string]any{
		"version":        float64(1),
		"request_id":     requestID.String(),
		"outcome":        ErasureOutcomeCompleted,
		"policy_version": "erasure-policy-2026-09",
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("payload[%q] = %v, want %v (payload %s)", key, payload[key], value, raw)
		}
	}
	if _, present := payload["completed_at"]; !present || len(payload) != len(want)+1 {
		t.Fatalf("payload has fields beyond the audit fact: %s", raw)
	}
	if event.AggregateID() != requestID || event.IdempotencyKey() != requestID {
		t.Fatal("the event is keyed by something other than the request")
	}
}

func TestAnErasureRecordNeedsANamedPolicy(t *testing.T) {
	for name, policy := range map[string]string{"empty": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			_, err := NewErasureCompletedEvent(uuid.New(), policy, time.Now())
			if !errors.Is(err, ErrErasurePolicyUnnamed) {
				t.Fatalf("NewErasureCompletedEvent(%q) error = %v, want ErrErasurePolicyUnnamed", policy, err)
			}
		})
	}
}

func TestAnErasureRecordRefusesWhatItCannotStore(t *testing.T) {
	for name, build := range map[string]func() (ErasureCompletedEvent, error){
		"no request": func() (ErasureCompletedEvent, error) { return NewErasureCompletedEvent(uuid.Nil, "p1", time.Now()) },
		"no time":    func() (ErasureCompletedEvent, error) { return NewErasureCompletedEvent(uuid.New(), "p1", time.Time{}) },
		"policy past bound": func() (ErasureCompletedEvent, error) {
			return NewErasureCompletedEvent(uuid.New(), strings.Repeat("p", 65), time.Now())
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := build(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestAPolicyLabelCannotBreakThePayload(t *testing.T) {
	// The old payload was built by string concatenation. That was safe only
	// because a UUID cannot contain a quote; an operator's policy label can.
	event, err := NewErasureCompletedEvent(uuid.New(), `v2", "customer_id": "leak`, time.Now())
	if err != nil {
		t.Fatalf("NewErasureCompletedEvent() error = %v", err)
	}
	raw, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if _, injected := payload["customer_id"]; injected {
		t.Fatalf("a policy label injected a field: %s", raw)
	}
}

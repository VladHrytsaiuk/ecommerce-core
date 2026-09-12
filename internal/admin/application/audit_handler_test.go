package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

func TestAdminActionEventSanitizesSensitivePayloadBeforeOutbox(t *testing.T) {
	event, err := adminDomain.NewAdminActionEvent(uuid.New(), uuid.New(), "promos.create", "promo", uuid.New(), []byte(`{"password":"old-secret","email":"buyer@example.com"}`), []byte(`{"nested":{"access_token":"new-secret","phone":"0501234567"},"code":"SAVE"}`), "127.0.0.1", []byte(`{"provider_secret":"not-for-outbox","authorization":"Bearer secret"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}

	// Each sensitive field is checked by value rather than by searching the
	// serialized document for its contents. A substring search was flaky: the
	// phone number under test was "123", and a randomly generated UUID
	// contains that about one run in fifty, failing a test whose subject had
	// redacted the field correctly.
	var decoded struct {
		OldPayload map[string]any `json:"old_payload"`
		NewPayload map[string]any `json:"new_payload"`
		Metadata   map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("outbox payload is not valid JSON: %v", err)
	}
	const redacted = "[REDACTED]"
	for name, check := range map[string]struct {
		field map[string]any
		key   string
	}{
		"old password":  {decoded.OldPayload, "password"},
		"old email":     {decoded.OldPayload, "email"},
		"provider key":  {decoded.Metadata, "provider_secret"},
		"authorization": {decoded.Metadata, "authorization"},
	} {
		if check.field[check.key] != redacted {
			t.Errorf("%s = %v, want %s", name, check.field[check.key], redacted)
		}
	}
	nested, ok := decoded.NewPayload["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested payload = %v", decoded.NewPayload["nested"])
	}
	if nested["access_token"] != redacted || nested["phone"] != redacted {
		t.Errorf("nested = %v, want both values redacted", nested)
	}
	// A non-sensitive field must survive, or "redact everything" would pass.
	if decoded.NewPayload["code"] != "SAVE" {
		t.Errorf("code = %v, want it preserved", decoded.NewPayload["code"])
	}
}

func TestAdminAuditHandlerSanitizesDefensively(t *testing.T) {
	repository := &recordingAuditRepository{}
	handler := NewAdminAuditEventHandler(repository)
	actorID, resourceID := uuid.New(), uuid.New()
	err := handler.Handle(context.Background(), events.Delivery{EventID: uuid.New(), Payload: []byte(`{"version":1,"actor_user_id":"` + actorID.String() + `","action":"promos.create","resource_type":"promo","resource_id":"` + resourceID.String() + `","new_payload":{"password":"unsafe"},"metadata":{"token":"unsafe"},"occurred_at":"2026-01-01T00:00:00Z"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(repository.event.NewPayload), "unsafe") || strings.Contains(string(repository.event.Metadata), "unsafe") {
		t.Fatalf("audit handler stored sensitive data: new=%s metadata=%s", repository.event.NewPayload, repository.event.Metadata)
	}
}

type recordingAuditRepository struct{ event adminDomain.AdminActionEvent }

func (r *recordingAuditRepository) Insert(_ context.Context, _ uuid.UUID, event adminDomain.AdminActionEvent) error {
	r.event = event
	return nil
}

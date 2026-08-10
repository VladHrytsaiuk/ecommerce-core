package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

func TestAdminActionEventSanitizesSensitivePayloadBeforeOutbox(t *testing.T) {
	event, err := adminDomain.NewAdminActionEvent(uuid.New(), uuid.New(), "promos.create", "promo", uuid.New(), []byte(`{"password":"old-secret","email":"buyer@example.com"}`), []byte(`{"nested":{"access_token":"new-secret","phone":"123"},"code":"SAVE"}`), "127.0.0.1", []byte(`{"provider_secret":"not-for-outbox","authorization":"Bearer secret"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := event.MarshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"old-secret", "new-secret", "not-for-outbox", "buyer@example.com", "123", "Bearer secret"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("outbox payload leaks %q: %s", secret, payload)
		}
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

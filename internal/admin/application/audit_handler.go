package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

type AuditLogRepository interface {
	Insert(context.Context, uuid.UUID, adminDomain.AdminActionEvent) error
}

type AdminAuditEventHandler struct{ repository AuditLogRepository }

func NewAdminAuditEventHandler(repository AuditLogRepository) *AdminAuditEventHandler {
	return &AdminAuditEventHandler{repository: repository}
}

func (h *AdminAuditEventHandler) Topic() string { return adminDomain.TopicAdminAction }

func (h *AdminAuditEventHandler) Handle(ctx context.Context, delivery events.Delivery) error {
	if h == nil || h.repository == nil || delivery.EventID == uuid.Nil {
		return fmt.Errorf("admin audit handler is not configured")
	}
	var payload struct {
		Version      int             `json:"version"`
		ActorUserID  uuid.UUID       `json:"actor_user_id"`
		Action       string          `json:"action"`
		ResourceType string          `json:"resource_type"`
		ResourceID   uuid.UUID       `json:"resource_id"`
		OldPayload   json.RawMessage `json:"old_payload"`
		NewPayload   json.RawMessage `json:"new_payload"`
		IPAddress    string          `json:"ip_address"`
		Metadata     json.RawMessage `json:"metadata"`
		OccurredAt   time.Time       `json:"occurred_at"`
	}
	if err := json.Unmarshal(delivery.Payload, &payload); err != nil || payload.Version != 1 {
		return fmt.Errorf("invalid admin audit event")
	}
	event, err := adminDomain.NewAdminActionEvent(delivery.EventID, payload.ActorUserID, payload.Action, payload.ResourceType, payload.ResourceID, payload.OldPayload, payload.NewPayload, payload.IPAddress, payload.Metadata, payload.OccurredAt)
	if err != nil {
		return fmt.Errorf("invalid admin audit event: %w", err)
	}
	return h.repository.Insert(ctx, delivery.EventID, event)
}

var _ eventsDomainConsumer = (*AdminAuditEventHandler)(nil)

// Local structural assertion keeps the handler coupled only to the event port.
type eventsDomainConsumer interface {
	Topic() string
	Handle(context.Context, events.Delivery) error
}

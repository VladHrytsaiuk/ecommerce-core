package domain

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

const TopicAdminAction = "admin.action.v1"

// AdminActionEvent is a minimal, immutable record of a privileged mutation.
// Payloads are sanitized at construction, before they enter the durable outbox.
type AdminActionEvent struct {
	EventKey     uuid.UUID
	ActorUserID  uuid.UUID
	Action       string
	ResourceType string
	ResourceID   uuid.UUID
	OldPayload   json.RawMessage
	NewPayload   json.RawMessage
	IPAddress    string
	Metadata     json.RawMessage
	At           time.Time
}

func NewAdminActionEvent(eventKey, actorID uuid.UUID, action, resourceType string, resourceID uuid.UUID, oldPayload, newPayload []byte, ip string, metadata []byte, occurredAt time.Time) (AdminActionEvent, error) {
	action, resourceType, ip = strings.TrimSpace(action), strings.TrimSpace(resourceType), strings.TrimSpace(ip)
	if eventKey == uuid.Nil || actorID == uuid.Nil || resourceID == uuid.Nil || action == "" || resourceType == "" {
		return AdminActionEvent{}, fmt.Errorf("invalid admin action event")
	}
	if ip != "" && net.ParseIP(ip) == nil {
		return AdminActionEvent{}, fmt.Errorf("invalid audit IP address")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return AdminActionEvent{EventKey: eventKey, ActorUserID: actorID, Action: action, ResourceType: resourceType, ResourceID: resourceID, OldPayload: SanitizeAuditPayload(oldPayload), NewPayload: SanitizeAuditPayload(newPayload), IPAddress: ip, Metadata: SanitizeAuditPayload(metadata), At: occurredAt.UTC()}, nil
}

func (AdminActionEvent) Topic() string               { return TopicAdminAction }
func (e AdminActionEvent) AggregateType() string     { return e.ResourceType }
func (e AdminActionEvent) AggregateID() uuid.UUID    { return e.ResourceID }
func (e AdminActionEvent) IdempotencyKey() uuid.UUID { return e.EventKey }
func (e AdminActionEvent) OccurredAt() time.Time     { return e.At }

func (e AdminActionEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version      int             `json:"version"`
		ActorUserID  uuid.UUID       `json:"actor_user_id"`
		Action       string          `json:"action"`
		ResourceType string          `json:"resource_type"`
		ResourceID   uuid.UUID       `json:"resource_id"`
		OldPayload   json.RawMessage `json:"old_payload,omitempty"`
		NewPayload   json.RawMessage `json:"new_payload,omitempty"`
		IPAddress    string          `json:"ip_address,omitempty"`
		Metadata     json.RawMessage `json:"metadata,omitempty"`
		OccurredAt   time.Time       `json:"occurred_at"`
	}{Version: 1, ActorUserID: e.ActorUserID, Action: e.Action, ResourceType: e.ResourceType, ResourceID: e.ResourceID, OldPayload: e.OldPayload, NewPayload: e.NewPayload, IPAddress: e.IPAddress, Metadata: e.Metadata, OccurredAt: e.At})
}

// SanitizeAuditPayload recursively redacts common credential-bearing fields.
// Invalid JSON is never persisted as an audit payload because it cannot be
// safely inspected or queried as JSONB.
func SanitizeAuditPayload(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	sanitizeValue(value)
	result, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return result
}

func sanitizeValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if sensitiveAuditField(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			sanitizeValue(child)
		}
	case []any:
		for _, child := range typed {
			sanitizeValue(child)
		}
	}
}

func sensitiveAuditField(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, sensitive := range []string{"password", "token", "secret", "email", "phone", "address", "api_key", "card", "cvv", "authorization"} {
		if strings.Contains(key, sensitive) {
			return true
		}
	}
	return false
}

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type AuditRepository struct{ db *gorm.DB }

func NewAuditRepository(db *gorm.DB) *AuditRepository { return &AuditRepository{db: db} }

type auditLogRecord struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	EventID      uuid.UUID `gorm:"type:uuid"`
	ActorUserID  uuid.UUID `gorm:"type:uuid"`
	Action       string
	ResourceType string
	ResourceID   uuid.UUID       `gorm:"type:uuid"`
	OldPayload   json.RawMessage `gorm:"type:jsonb"`
	NewPayload   json.RawMessage `gorm:"type:jsonb"`
	IPAddress    *string         `gorm:"type:inet"`
	Metadata     json.RawMessage `gorm:"type:jsonb"`
	OccurredAt   time.Time
}

func (auditLogRecord) TableName() string { return "audit_logs" }

// Insert ignores a repeated event ID, making an at-least-once delivery safe.
func (r *AuditRepository) Insert(ctx context.Context, eventID uuid.UUID, event adminDomain.AdminActionEvent) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("audit repository is not configured")
	}
	var ip *string
	if event.IPAddress != "" {
		ip = &event.IPAddress
	}
	metadata := event.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	record := auditLogRecord{ID: uuid.New(), EventID: eventID, ActorUserID: event.ActorUserID, Action: event.Action, ResourceType: event.ResourceType, ResourceID: event.ResourceID, OldPayload: event.OldPayload, NewPayload: event.NewPayload, IPAddress: ip, Metadata: metadata, OccurredAt: event.At}
	db := r.db.WithContext(ctx)
	if tx, err := transaction.FromContext(ctx); err == nil {
		db = tx.WithContext(ctx)
	}
	return db.Exec(`INSERT INTO audit_logs (id, event_id, actor_user_id, action, resource_type, resource_id, old_payload, new_payload, ip_address, metadata, occurred_at) VALUES (?, ?, ?, ?, ?, ?, ?::jsonb, ?::jsonb, ?::inet, ?::jsonb, ?) ON CONFLICT (event_id) DO NOTHING`, record.ID, record.EventID, record.ActorUserID, record.Action, record.ResourceType, record.ResourceID, nullableJSON(record.OldPayload), nullableJSON(record.NewPayload), record.IPAddress, string(record.Metadata), record.OccurredAt).Error
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

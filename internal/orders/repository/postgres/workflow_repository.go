package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// WorkflowRepository is the PostgreSQL adapter for module-owned workflow
// configuration. It deliberately does not update core.orders; the cross-core
// OrderWorkflow repository remains the transaction owner for that operation.
type WorkflowRepository struct {
	db *gorm.DB
}

func NewWorkflowRepository(db *gorm.DB) *WorkflowRepository {
	return &WorkflowRepository{db: db}
}

type statusDefinitionRecord struct {
	Code          string
	Name          string
	Description   string
	Color         string
	SortOrder     int
	Kind          string
	IsInitial     bool
	IsTerminal    bool
	SystemManaged bool
	CustomerLabel string
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (statusDefinitionRecord) TableName() string { return "order_status_definitions" }

type statusTransitionRecord struct {
	ID                     uuid.UUID
	FromStatusCode         string
	ToStatusCode           string
	AllowedTriggers        pq.StringArray `gorm:"type:text[]"`
	RequiresPayment        bool
	RequiresTrackingNumber bool
	RequiresReason         bool
	RequiredPermission     string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (statusTransitionRecord) TableName() string { return "order_status_transitions" }

type statusHistoryRecord struct {
	ID             uuid.UUID
	OrderID        uuid.UUID
	FromStatusCode *string
	ToStatusCode   string
	ActorType      string
	ActorID        *uuid.UUID
	Reason         *string
	Metadata       json.RawMessage `gorm:"type:jsonb"`
	EventID        *uuid.UUID
	OccurredAt     time.Time
	CreatedAt      time.Time
}

func (statusHistoryRecord) TableName() string { return "order_status_history" }

func (r *WorkflowRepository) LoadTransitionPolicy(ctx context.Context, fromCode, toCode string) (domain.TransitionPolicy, error) {
	db := r.database(ctx).Clauses(clause.Locking{Strength: "SHARE"})
	var from, to statusDefinitionRecord
	if err := db.First(&from, "code = ?", fromCode).Error; err != nil {
		return domain.TransitionPolicy{}, workflowNotFound(err)
	}
	if err := db.First(&to, "code = ?", toCode).Error; err != nil {
		return domain.TransitionPolicy{}, workflowNotFound(err)
	}
	var transition statusTransitionRecord
	if err := db.First(&transition, "from_status_code = ? AND to_status_code = ?", fromCode, toCode).Error; err != nil {
		return domain.TransitionPolicy{}, workflowNotFound(err)
	}
	triggers := make([]domain.TransitionTrigger, len(transition.AllowedTriggers))
	for i, trigger := range transition.AllowedTriggers {
		triggers[i] = domain.TransitionTrigger(trigger)
	}
	return domain.TransitionPolicy{
		From: mapStatusDefinition(from),
		To:   mapStatusDefinition(to),
		Transition: domain.OrderStatusTransition{
			ID:                     transition.ID,
			FromStatusCode:         transition.FromStatusCode,
			ToStatusCode:           transition.ToStatusCode,
			AllowedTriggers:        triggers,
			RequiresPayment:        transition.RequiresPayment,
			RequiresTrackingNumber: transition.RequiresTrackingNumber,
			RequiresReason:         transition.RequiresReason,
			RequiredPermission:     transition.RequiredPermission,
		},
	}, nil
}

func (r *WorkflowRepository) ListWorkflowConfiguration(ctx context.Context) ([]domain.OrderStatusDefinition, []domain.OrderStatusTransition, error) {
	db := r.database(ctx)
	var definitions []statusDefinitionRecord
	if err := db.Order("sort_order ASC, code ASC").Find(&definitions).Error; err != nil {
		return nil, nil, err
	}
	var records []statusTransitionRecord
	if err := db.Order("from_status_code ASC, to_status_code ASC").Find(&records).Error; err != nil {
		return nil, nil, err
	}
	statuses := make([]domain.OrderStatusDefinition, 0, len(definitions))
	for _, definition := range definitions {
		statuses = append(statuses, mapStatusDefinition(definition))
	}
	transitions := make([]domain.OrderStatusTransition, 0, len(records))
	for _, record := range records {
		triggers := make([]domain.TransitionTrigger, len(record.AllowedTriggers))
		for i, trigger := range record.AllowedTriggers {
			triggers[i] = domain.TransitionTrigger(trigger)
		}
		transitions = append(transitions, domain.OrderStatusTransition{
			ID: record.ID, FromStatusCode: record.FromStatusCode, ToStatusCode: record.ToStatusCode,
			AllowedTriggers: triggers, RequiresPayment: record.RequiresPayment,
			RequiresTrackingNumber: record.RequiresTrackingNumber, RequiresReason: record.RequiresReason,
			RequiredPermission: record.RequiredPermission, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		})
	}
	return statuses, transitions, nil
}

func (r *WorkflowRepository) ListStatusHistory(ctx context.Context, orderID uuid.UUID) ([]domain.OrderStatusHistory, error) {
	var records []statusHistoryRecord
	if err := r.database(ctx).
		Where("order_id = ?", orderID).
		Order("occurred_at DESC, id DESC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	history := make([]domain.OrderStatusHistory, 0, len(records))
	for _, record := range records {
		history = append(history, mapStatusHistory(record))
	}
	return history, nil
}

func (r *WorkflowRepository) SaveStatusDefinition(ctx context.Context, definition domain.OrderStatusDefinition) error {
	result := r.database(ctx).Exec(`
INSERT INTO order_status_definitions
    (code, name, description, color, sort_order, kind, is_initial, is_terminal, system_managed, customer_label, enabled)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE, ?, ?)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name, description = EXCLUDED.description, color = EXCLUDED.color,
    sort_order = EXCLUDED.sort_order, kind = EXCLUDED.kind, is_initial = EXCLUDED.is_initial,
    is_terminal = EXCLUDED.is_terminal, customer_label = EXCLUDED.customer_label,
    enabled = EXCLUDED.enabled, updated_at = CURRENT_TIMESTAMP
WHERE order_status_definitions.system_managed = FALSE`,
		definition.Code, definition.Name, definition.Description, definition.Color, definition.SortOrder,
		string(definition.Kind), definition.IsInitial, definition.IsTerminal, definition.CustomerLabel, definition.Enabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrWorkflowSystemManaged
	}
	return nil
}

func (r *WorkflowRepository) SaveStatusTransition(ctx context.Context, transition domain.OrderStatusTransition) error {
	triggers := make(pq.StringArray, len(transition.AllowedTriggers))
	for i, trigger := range transition.AllowedTriggers {
		triggers[i] = string(trigger)
	}
	result := r.database(ctx).Exec(`
INSERT INTO order_status_transitions
    (id, from_status_code, to_status_code, allowed_triggers, requires_payment, requires_tracking_number, requires_reason, required_permission)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (from_status_code, to_status_code) DO UPDATE SET
    allowed_triggers = EXCLUDED.allowed_triggers,
    requires_payment = EXCLUDED.requires_payment,
    requires_tracking_number = EXCLUDED.requires_tracking_number,
    requires_reason = EXCLUDED.requires_reason,
    required_permission = EXCLUDED.required_permission,
    updated_at = CURRENT_TIMESTAMP`,
		uuid.New(), transition.FromStatusCode, transition.ToStatusCode, triggers,
		transition.RequiresPayment, transition.RequiresTrackingNumber, transition.RequiresReason, transition.RequiredPermission)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("save order status transition")
	}
	return nil
}

func (r *WorkflowRepository) AppendStatusHistory(ctx context.Context, history domain.OrderStatusHistory) (bool, error) {
	metadata := history.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(metadata) {
		return false, fmt.Errorf("invalid order status history metadata")
	}
	record := map[string]any{
		"id":               history.ID,
		"order_id":         history.OrderID,
		"from_status_code": nullableString(history.FromStatusCode),
		"to_status_code":   history.ToStatusCode,
		"actor_type":       string(history.ActorType),
		"actor_id":         history.ActorID,
		"reason":           nullableString(history.Reason),
		"metadata":         string(metadata),
		"event_id":         history.EventID,
		"occurred_at":      history.OccurredAt,
	}
	if history.ID == uuid.Nil {
		record["id"] = uuid.New()
	}
	result := r.database(ctx).Table("order_status_history").Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *WorkflowRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func mapStatusDefinition(record statusDefinitionRecord) domain.OrderStatusDefinition {
	return domain.OrderStatusDefinition{
		Code: record.Code, Name: record.Name, Description: record.Description, Color: record.Color,
		SortOrder: record.SortOrder, Kind: domain.StatusKind(record.Kind), IsInitial: record.IsInitial,
		IsTerminal: record.IsTerminal, SystemManaged: record.SystemManaged,
		CustomerLabel: record.CustomerLabel, Enabled: record.Enabled,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func mapStatusHistory(record statusHistoryRecord) domain.OrderStatusHistory {
	history := domain.OrderStatusHistory{
		ID: record.ID, OrderID: record.OrderID, ToStatusCode: record.ToStatusCode,
		ActorType: domain.StatusActorType(record.ActorType), ActorID: record.ActorID,
		Metadata: record.Metadata, EventID: record.EventID, OccurredAt: record.OccurredAt,
		CreatedAt: record.CreatedAt,
	}
	if record.FromStatusCode != nil {
		history.FromStatusCode = *record.FromStatusCode
	}
	if record.Reason != nil {
		history.Reason = *record.Reason
	}
	if len(history.Metadata) == 0 {
		history.Metadata = json.RawMessage(`{}`)
	}
	return history
}

func workflowNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrWorkflowNotFound
	}
	return err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ domain.WorkflowRepository = (*WorkflowRepository)(nil)
var _ domain.WorkflowConfigurationRepository = (*WorkflowRepository)(nil)

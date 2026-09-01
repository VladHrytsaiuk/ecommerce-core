package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TopicStatusChanged       = "returns.status_changed.v1"
	TopicSettlementRequested = "returns.settlement_requested.v1"
	ConsumerSettlement       = "returns_settlement"
)

// StatusChangedEvent is the minimal immutable audit/integration contract for
// a state transition. It deliberately contains no reason, product, address or
// other PII.
type StatusChangedEvent struct {
	EventID, ReturnID uuid.UUID
	FromStatus        ReturnStatus
	ToStatus          ReturnStatus
	ActorType         ActorType
	Occurred          time.Time
}

func NewStatusChangedEvent(eventID, returnID uuid.UUID, from, to ReturnStatus, actorType ActorType, occurred time.Time) (StatusChangedEvent, error) {
	if eventID == uuid.Nil || returnID == uuid.Nil || !isValidReturnStatus(from) || !isValidReturnStatus(to) || !isValidActorType(actorType) || occurred.IsZero() {
		return StatusChangedEvent{}, fmt.Errorf("invalid return status changed event")
	}
	return StatusChangedEvent{EventID: eventID, ReturnID: returnID, FromStatus: from, ToStatus: to, ActorType: actorType, Occurred: occurred.UTC()}, nil
}
func (StatusChangedEvent) Topic() string               { return TopicStatusChanged }
func (StatusChangedEvent) AggregateType() string       { return "return_request" }
func (e StatusChangedEvent) AggregateID() uuid.UUID    { return e.ReturnID }
func (e StatusChangedEvent) IdempotencyKey() uuid.UUID { return e.EventID }
func (e StatusChangedEvent) OccurredAt() time.Time     { return e.Occurred }
func (e StatusChangedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version    int          `json:"version"`
		ReturnID   uuid.UUID    `json:"return_id"`
		FromStatus ReturnStatus `json:"from_status"`
		ToStatus   ReturnStatus `json:"to_status"`
		ActorType  ActorType    `json:"actor_type"`
		OccurredAt time.Time    `json:"occurred_at"`
	}{Version: 1, ReturnID: e.ReturnID, FromStatus: e.FromStatus, ToStatus: e.ToStatus, ActorType: e.ActorType, OccurredAt: e.Occurred})
}

// SettlementRequestedEvent is a durable command for the worker that performs
// restock and provider I/O after the received transition has committed.
type SettlementRequestedEvent struct {
	EventID, ReturnID, OrderID uuid.UUID
	Occurred                   time.Time
}

func NewSettlementRequestedEvent(eventID, returnID, orderID uuid.UUID, occurred time.Time) (SettlementRequestedEvent, error) {
	if eventID == uuid.Nil || returnID == uuid.Nil || orderID == uuid.Nil || occurred.IsZero() {
		return SettlementRequestedEvent{}, fmt.Errorf("invalid return settlement event")
	}
	return SettlementRequestedEvent{EventID: eventID, ReturnID: returnID, OrderID: orderID, Occurred: occurred.UTC()}, nil
}
func (SettlementRequestedEvent) Topic() string               { return TopicSettlementRequested }
func (SettlementRequestedEvent) AggregateType() string       { return "return_request" }
func (e SettlementRequestedEvent) AggregateID() uuid.UUID    { return e.ReturnID }
func (e SettlementRequestedEvent) IdempotencyKey() uuid.UUID { return e.EventID }
func (e SettlementRequestedEvent) OccurredAt() time.Time     { return e.Occurred }
func (e SettlementRequestedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version    int       `json:"version"`
		ReturnID   uuid.UUID `json:"return_id"`
		OrderID    uuid.UUID `json:"order_id"`
		OccurredAt time.Time `json:"occurred_at"`
	}{Version: 1, ReturnID: e.ReturnID, OrderID: e.OrderID, OccurredAt: e.Occurred})
}

func isValidActorType(actorType ActorType) bool {
	return actorType == ActorTypeSystem || actorType == ActorTypeAdmin || actorType == ActorTypeCustomer
}

func NormalizeReason(reason string) string { return strings.TrimSpace(reason) }

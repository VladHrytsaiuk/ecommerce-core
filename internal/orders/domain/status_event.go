package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TopicOrderStatusChanged is the stable, provider-neutral contract emitted
// only after a status mutation and its immutable history entry commit in the
// same transaction. It deliberately contains no payment, customer or address
// data.
const TopicOrderStatusChanged = "orders.status_changed.v1"

// OrderStatusChangedEvent is a versioned cross-module notification of one
// committed lifecycle movement. TransitionID is an internal idempotency key;
// it is intentionally not part of the public payload because domain_events
// already assigns the delivery event ID.
type OrderStatusChangedEvent struct {
	OrderID      uuid.UUID       `json:"order_id"`
	FromStatus   string          `json:"from_status"`
	ToStatus     string          `json:"to_status"`
	ActorType    StatusActorType `json:"actor_type"`
	At           time.Time       `json:"occurred_at"`
	Version      int             `json:"version"`
	transitionID uuid.UUID
}

func NewOrderStatusChangedEvent(orderID uuid.UUID, fromStatus, toStatus string, actorType StatusActorType, occurredAt time.Time, transitionID uuid.UUID) (OrderStatusChangedEvent, error) {
	fromStatus = strings.TrimSpace(fromStatus)
	toStatus = strings.TrimSpace(toStatus)
	if orderID == uuid.Nil || toStatus == "" || fromStatus == toStatus ||
		!validStatusActorType(actorType) || transitionID == uuid.Nil {
		return OrderStatusChangedEvent{}, fmt.Errorf("invalid order status changed event")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return OrderStatusChangedEvent{
		OrderID: orderID, FromStatus: fromStatus, ToStatus: toStatus,
		ActorType: actorType, At: occurredAt.UTC(), Version: 1,
		transitionID: transitionID,
	}, nil
}

func (OrderStatusChangedEvent) Topic() string            { return TopicOrderStatusChanged }
func (OrderStatusChangedEvent) AggregateType() string    { return "order" }
func (e OrderStatusChangedEvent) AggregateID() uuid.UUID { return e.OrderID }
func (e OrderStatusChangedEvent) IdempotencyKey() uuid.UUID {
	return e.transitionID
}

// OccurredAt implements the Core Outbox event contract without importing the
// Core events package into this domain module.
func (e OrderStatusChangedEvent) OccurredAt() time.Time { return e.At }

func (e OrderStatusChangedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		OrderID    uuid.UUID       `json:"order_id"`
		FromStatus string          `json:"from_status"`
		ToStatus   string          `json:"to_status"`
		ActorType  StatusActorType `json:"actor_type"`
		OccurredAt time.Time       `json:"occurred_at"`
		Version    int             `json:"version"`
	}{e.OrderID, e.FromStatus, e.ToStatus, e.ActorType, e.At, e.Version})
}

func validStatusActorType(actorType StatusActorType) bool {
	switch actorType {
	case StatusActorAdmin, StatusActorSystem, StatusActorPaymentWebhook, StatusActorDeliveryWebhook, StatusActorDeliveryProvider, StatusActorCustomer:
		return true
	default:
		return false
	}
}

// Package events defines durable, provider-neutral domain event contracts.
// PostgreSQL persistence and worker scheduling remain infrastructure adapters.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

const (
	ConsumerNotifications = "notifications"
	// ConsumerAdminAudit owns immutable records of privileged back-office
	// mutations. It deliberately has its own delivery so a notification failure
	// cannot delay or suppress an audit trail.
	ConsumerAdminAudit = "admin_audit"
	// ConsumerSearchIndexer owns the asynchronous projection of Catalog
	// products into an optional external search engine.
	ConsumerSearchIndexer = "search_indexer"
	// ConsumerMediaProcessor owns asynchronous media state transitions.
	ConsumerMediaProcessor = "media_processor"
	TopicOrderPaid         = "orders.paid.v1"
)

// DomainEvent is an immutable versioned message. Its payload must contain
// only the minimum data required by the subscribed workflow.
type DomainEvent interface {
	Topic() string
	AggregateType() string
	AggregateID() uuid.UUID
	IdempotencyKey() uuid.UUID
	OccurredAt() time.Time
	MarshalPayload() ([]byte, error)
}

// TransactionalEventPublisher appends an event and its configured deliveries
// to the transaction carried by ctx. Implementations must not do external I/O.
type TransactionalEventPublisher interface {
	Publish(context.Context, DomainEvent) error
}

// Delivery is the leased work item consumed by a background worker.
type Delivery struct {
	EventID       uuid.UUID
	Topic         string
	AggregateType string
	AggregateID   uuid.UUID
	Payload       []byte
	Consumer      string
	Attempts      int
	OccurredAt    time.Time
}

// DeliveryStore owns the lease lifecycle for a consumer-specific delivery.
type DeliveryStore interface {
	Claim(context.Context, string, time.Time, time.Duration) (*Delivery, error)
	Complete(context.Context, uuid.UUID, string, time.Time) error
	Fail(context.Context, uuid.UUID, string, error, time.Time) error
	Dead(context.Context, uuid.UUID, string, error, time.Time) error
}

// OrderPaidEvent is emitted exactly once per paid order by its stable order ID.
type OrderPaidEvent struct {
	OrderID     uuid.UUID
	OrderNumber string
	Total       money.Money
	PaidAt      time.Time
}

func NewOrderPaidEvent(orderID uuid.UUID, orderNumber string, total money.Money, paidAt time.Time) (OrderPaidEvent, error) {
	if orderID == uuid.Nil || strings.TrimSpace(orderNumber) == "" || total.Amount < 0 || strings.TrimSpace(total.Currency) == "" {
		return OrderPaidEvent{}, fmt.Errorf("invalid order paid event")
	}
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	return OrderPaidEvent{OrderID: orderID, OrderNumber: strings.TrimSpace(orderNumber), Total: total, PaidAt: paidAt.UTC()}, nil
}

func (OrderPaidEvent) Topic() string               { return TopicOrderPaid }
func (OrderPaidEvent) AggregateType() string       { return "order" }
func (e OrderPaidEvent) AggregateID() uuid.UUID    { return e.OrderID }
func (e OrderPaidEvent) IdempotencyKey() uuid.UUID { return e.OrderID }
func (e OrderPaidEvent) OccurredAt() time.Time     { return e.PaidAt }

func (e OrderPaidEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version     int       `json:"version"`
		OrderID     uuid.UUID `json:"order_id"`
		OrderNumber string    `json:"order_number"`
		TotalAmount int64     `json:"total_amount"`
		Currency    string    `json:"currency"`
		PaidAt      time.Time `json:"paid_at"`
	}{Version: 1, OrderID: e.OrderID, OrderNumber: e.OrderNumber, TotalAmount: e.Total.Amount, Currency: e.Total.Currency, PaidAt: e.PaidAt})
}

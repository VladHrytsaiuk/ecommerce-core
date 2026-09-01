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
	// ConsumerReportsProjection owns idempotent CQRS aggregates for the admin
	// business-analytics dashboard.
	ConsumerReportsProjection = "reports_projection"
	TopicOrderPaid            = "orders.paid.v1"
	TopicOrderRefunded        = "orders.refunded.v1"
	TopicCartCreated          = "carts.created.v1"
	TopicCheckoutStarted      = "checkout.started.v1"
	TopicCartUpdated          = "cart.updated.v1"
	// TopicCheckoutEmailCaptured deliberately contains no address or other PII.
	// Consumers resolve the current contact through a narrowly scoped port.
	TopicCheckoutEmailCaptured = "checkout.email_captured.v1"
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

// CheckoutEmailCapturedEvent is emitted after the contact snapshot has been
// committed. The payload is intentionally limited to the cart identity.
type CheckoutEmailCapturedEvent struct {
	CartID uuid.UUID
	At     time.Time
}

func NewCheckoutEmailCapturedEvent(cartID uuid.UUID, at time.Time) (CheckoutEmailCapturedEvent, error) {
	if cartID == uuid.Nil {
		return CheckoutEmailCapturedEvent{}, fmt.Errorf("invalid checkout email captured event")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return CheckoutEmailCapturedEvent{CartID: cartID, At: at.UTC()}, nil
}
func (CheckoutEmailCapturedEvent) Topic() string            { return TopicCheckoutEmailCaptured }
func (CheckoutEmailCapturedEvent) AggregateType() string    { return "checkout_contact" }
func (e CheckoutEmailCapturedEvent) AggregateID() uuid.UUID { return e.CartID }
func (e CheckoutEmailCapturedEvent) IdempotencyKey() uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(e.CartID.String()+":"+e.At.Format(time.RFC3339Nano)))
}
func (e CheckoutEmailCapturedEvent) OccurredAt() time.Time { return e.At }
func (e CheckoutEmailCapturedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version int       `json:"version"`
		CartID  uuid.UUID `json:"cart_id"`
	}{Version: 1, CartID: e.CartID})
}

type CartUpdatedEvent struct {
	CartID     uuid.UUID
	CustomerID *uuid.UUID
	At         time.Time
}

func NewCartUpdatedEvent(cartID uuid.UUID, customerID *uuid.UUID, at time.Time) (CartUpdatedEvent, error) {
	if cartID == uuid.Nil {
		return CartUpdatedEvent{}, fmt.Errorf("invalid cart updated event")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return CartUpdatedEvent{cartID, customerID, at.UTC()}, nil
}
func (CartUpdatedEvent) Topic() string            { return TopicCartUpdated }
func (CartUpdatedEvent) AggregateType() string    { return "cart" }
func (e CartUpdatedEvent) AggregateID() uuid.UUID { return e.CartID }
func (e CartUpdatedEvent) IdempotencyKey() uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(e.CartID.String()+e.At.Format(time.RFC3339Nano)))
}
func (e CartUpdatedEvent) OccurredAt() time.Time { return e.At }
func (e CartUpdatedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version    int        `json:"version"`
		CartID     uuid.UUID  `json:"cart_id"`
		CustomerID *uuid.UUID `json:"customer_id,omitempty"`
	}{1, e.CartID, e.CustomerID})
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
	TraceParent   string
	TraceState    string
	RequestID     string
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
	if orderID == uuid.Nil || strings.TrimSpace(orderNumber) == "" || total.Validate() != nil {
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
	}{Version: 1, OrderID: e.OrderID, OrderNumber: e.OrderNumber, TotalAmount: e.Total.Amount(), Currency: e.Total.Currency(), PaidAt: e.PaidAt})
}

// OrderRefundedEvent is emitted after a verified full refund committed locally.
type OrderRefundedEvent struct {
	OrderID uuid.UUID
	Total   money.Money
	At      time.Time
}

func NewOrderRefundedEvent(orderID uuid.UUID, total money.Money, occurredAt time.Time) (OrderRefundedEvent, error) {
	if orderID == uuid.Nil || total.Validate() != nil {
		return OrderRefundedEvent{}, fmt.Errorf("invalid order refunded event")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return OrderRefundedEvent{OrderID: orderID, Total: total, At: occurredAt.UTC()}, nil
}
func (OrderRefundedEvent) Topic() string               { return TopicOrderRefunded }
func (OrderRefundedEvent) AggregateType() string       { return "order" }
func (e OrderRefundedEvent) AggregateID() uuid.UUID    { return e.OrderID }
func (e OrderRefundedEvent) IdempotencyKey() uuid.UUID { return e.OrderID }
func (e OrderRefundedEvent) OccurredAt() time.Time     { return e.At }
func (e OrderRefundedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version    int       `json:"version"`
		OrderID    uuid.UUID `json:"order_id"`
		Amount     int64     `json:"amount_minor"`
		Currency   string    `json:"currency"`
		RefundedAt time.Time `json:"refunded_at"`
	}{1, e.OrderID, e.Total.Amount(), e.Total.Currency(), e.At})
}

// CartCreatedEvent and CheckoutStartedEvent contain no customer, contact, or
// payment data; they are solely anonymous funnel counters.
type CartCreatedEvent struct {
	CartID uuid.UUID
	At     time.Time
}

func NewCartCreatedEvent(cartID uuid.UUID, occurredAt time.Time) (CartCreatedEvent, error) {
	if cartID == uuid.Nil {
		return CartCreatedEvent{}, fmt.Errorf("invalid cart created event")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return CartCreatedEvent{CartID: cartID, At: occurredAt.UTC()}, nil
}
func (CartCreatedEvent) Topic() string               { return TopicCartCreated }
func (CartCreatedEvent) AggregateType() string       { return "cart" }
func (e CartCreatedEvent) AggregateID() uuid.UUID    { return e.CartID }
func (e CartCreatedEvent) IdempotencyKey() uuid.UUID { return e.CartID }
func (e CartCreatedEvent) OccurredAt() time.Time     { return e.At }
func (e CartCreatedEvent) MarshalPayload() ([]byte, error) {
	return json.Marshal(struct {
		Version   int       `json:"version"`
		CartID    uuid.UUID `json:"cart_id"`
		CreatedAt time.Time `json:"created_at"`
	}{1, e.CartID, e.At})
}

type CheckoutStartedEvent struct {
	OrderID, CartID uuid.UUID
	At              time.Time
}

func NewCheckoutStartedEvent(orderID, cartID uuid.UUID, occurredAt time.Time) (CheckoutStartedEvent, error) {
	if orderID == uuid.Nil {
		return CheckoutStartedEvent{}, fmt.Errorf("invalid checkout started event")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return CheckoutStartedEvent{OrderID: orderID, CartID: cartID, At: occurredAt.UTC()}, nil
}
func (CheckoutStartedEvent) Topic() string               { return TopicCheckoutStarted }
func (CheckoutStartedEvent) AggregateType() string       { return "checkout" }
func (e CheckoutStartedEvent) AggregateID() uuid.UUID    { return e.OrderID }
func (e CheckoutStartedEvent) IdempotencyKey() uuid.UUID { return e.OrderID }
func (e CheckoutStartedEvent) OccurredAt() time.Time     { return e.At }
func (e CheckoutStartedEvent) MarshalPayload() ([]byte, error) {
	var cartID *uuid.UUID
	if e.CartID != uuid.Nil {
		cartID = &e.CartID
	}
	return json.Marshal(struct {
		Version   int        `json:"version"`
		OrderID   uuid.UUID  `json:"order_id"`
		CartID    *uuid.UUID `json:"cart_id,omitempty"`
		StartedAt time.Time  `json:"started_at"`
	}{1, e.OrderID, cartID, e.At})
}

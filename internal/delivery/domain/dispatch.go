package domain

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/google/uuid"
)

// DispatchJob is a durable, provider-neutral work item. Repository claiming is
// transactional; carrier I/O is intentionally performed after Claim returns.
type DispatchJob struct {
	ID             uuid.UUID
	OrderID        uuid.UUID
	Provider       string
	IdempotencyKey uuid.UUID
	Destination    Address
	Items          []ShipmentItem
	DeclaredValue  money.Money
	// Attempts includes the claim which returned this job.
	Attempts int
}

type JobStore interface {
	Claim(context.Context, time.Time) (*DispatchJob, error)
	Complete(context.Context, uuid.UUID, ShipmentResult) error
	Retry(context.Context, uuid.UUID, error, time.Time) error
	Fail(context.Context, uuid.UUID, error) error
	Dead(context.Context, uuid.UUID, error) error
}

type TrackingDelivery struct {
	ID, OrderID    uuid.UUID
	Provider       string
	TrackingNumber string
	RecipientPhone string
	Status         string
}

// OrderStatusTransition is a narrow bridge port payload. Delivery owns no
// Orders model and only supplies facts independently verified by the carrier.
type OrderStatusTransition struct {
	DeliveryID     uuid.UUID
	OrderID        uuid.UUID
	ToStatusCode   string
	TrackingNumber string
	OccurredAt     time.Time
}

// OrderTransitioner is implemented by an adapter around the Orders workflow.
// Its invocation happens from TrackingStore's transaction callback, so the
// order and delivery mutation share one context-bound SQL transaction.
type OrderTransitioner interface {
	TransitionFromDelivery(context.Context, OrderStatusTransition) error
}

type TrackingStore interface {
	ListActive(context.Context, int) ([]TrackingDelivery, error)
	UpdateStatusAndTransition(context.Context, TrackingDelivery, TrackingResult, OrderTransitioner) error
}

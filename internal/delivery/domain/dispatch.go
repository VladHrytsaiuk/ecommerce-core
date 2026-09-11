package domain

import (
	"context"
	"errors"
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
	// LockToken identifies this claim. Every terminal write presents it, so a
	// worker whose lease was reclaimed while it was still running cannot
	// overwrite the shipment a newer claim produced.
	LockToken uuid.UUID
	// LastFailureWasDefinite reports that the previous attempt is known to have
	// created nothing. It defaults to false, so an unrecorded or unknown
	// failure is treated as possibly having reached the carrier.
	LastFailureWasDefinite bool
}

// JobStore takes the whole claimed job rather than its id so a caller cannot
// write a terminal state without the token that authorizes it.
type JobStore interface {
	Claim(context.Context, time.Time) (*DispatchJob, error)
	Complete(context.Context, DispatchJob, ShipmentResult) error
	// Retry schedules another attempt. The final argument records whether
	// this failure is known to have created no shipment.
	Retry(context.Context, DispatchJob, error, time.Time, bool) error
	Fail(context.Context, DispatchJob, error) error
	Dead(context.Context, DispatchJob, error) error
}

// ErrLeaseLost reports that a job was reclaimed by another worker before this
// one finished. The work this worker did is not lost silently: the newer claim
// owns the job, and a carrier that can reconcile will find whatever was
// created. Callers must not retry against the same claim.
var ErrLeaseLost = errors.New("delivery job lease was lost to another worker")

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

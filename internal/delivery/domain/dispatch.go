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
}

type JobStore interface {
	Claim(context.Context, time.Time) (*DispatchJob, error)
	Complete(context.Context, uuid.UUID, ShipmentResult) error
	Retry(context.Context, uuid.UUID, error, time.Time) error
	Fail(context.Context, uuid.UUID, error) error
}

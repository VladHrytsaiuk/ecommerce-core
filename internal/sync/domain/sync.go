// Package domain defines provider-neutral Sync contracts. Concrete ERP
// protocols belong in adapters, never in Catalog, Inventory or Checkout.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const TopicOrderCreated = "order.created"

var ErrInboundVersionConflict = errors.New("sync inbound version conflicts with an existing payload")

// OutboxEvent is a durable integration message. Payload is a versioned
// transport envelope whose schema is defined by its topic.
type OutboxEvent struct {
	ID             uuid.UUID
	Topic          string
	AggregateID    uuid.UUID
	IdempotencyKey uuid.UUID
	Payload        []byte
	Attempts       int
	CreatedAt      time.Time
	// LockedAt fences this claim. A dispatcher that overruns its lease has the
	// event re-claimed by another, which leaves the status at 'processing';
	// without this token the slow dispatcher's acknowledgement would still
	// match and would finalize an export the new holder is still performing.
	LockedAt time.Time
}

// ErrLeaseLost reports that an event was re-claimed by another dispatcher
// before this one finished. The export is not lost — its new holder owns it —
// so this is an expected race, not an infrastructure fault.
var ErrLeaseLost = errors.New("sync outbox lease was lost")

// OutboxStore claims one due message exclusively. A claimed event remains
// recoverable after lease expiry if its worker crashes. Every finalizing call
// takes the lockedAt token returned by Claim and applies only while the event
// still holds that exact lease.
type OutboxStore interface {
	Claim(context.Context, time.Time, time.Duration) (*OutboxEvent, error)
	Complete(ctx context.Context, eventID uuid.UUID, lockedAt, deliveredAt time.Time) error
	Retry(ctx context.Context, eventID uuid.UUID, cause error, lockedAt, availableAt time.Time) error
	DeadLetter(ctx context.Context, eventID uuid.UUID, cause error, lockedAt, deadAt time.Time) error
}

// OrderExporter is implemented by an ERP adapter. Its implementation must
// forward IdempotencyKey to the remote provider and treat duplicate delivery
// as success.
type OrderExporter interface {
	ExportOrder(context.Context, OutboxEvent) error
}

// StockChange is an authenticated adapter's normalized ERP snapshot. Payload
// hash must be the SHA-256 of the verified provider payload, not browser data.
type StockChange struct {
	Source          string
	ExternalID      string
	Version         string
	SourceUpdatedAt time.Time
	PayloadHash     string
	VariantID       uuid.UUID
	WarehouseID     uuid.UUID
	Quantity        int
}

// InboundStateStore makes source/version/hash idempotency durable. Claiming a
// change is separate from applying the read model so an expired lease can
// safely recover a process crash.
type InboundStateStore interface {
	ClaimStockChange(context.Context, StockChange, time.Time, time.Duration) (bool, error)
	MarkStockChangeApplied(context.Context, StockChange) error
	MarkStockChangeFailed(context.Context, StockChange, error) error
}

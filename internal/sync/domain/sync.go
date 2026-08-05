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
}

// OutboxStore claims one due message exclusively. A claimed event remains
// recoverable after lease expiry if its worker crashes.
type OutboxStore interface {
	Claim(context.Context, time.Time, time.Duration) (*OutboxEvent, error)
	Complete(context.Context, uuid.UUID, time.Time) error
	Retry(context.Context, uuid.UUID, error, time.Time) error
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

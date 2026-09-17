package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

const TopicVariantAvailable = "inventory.variant.available.v1"

// AvailabilityAdjustment is returned by storage atomically with a stock
// mutation; it prevents a read-then-write 0->positive race.
type AvailabilityAdjustment interface {
	AdjustAndReportAvailability(context.Context, uuid.UUID, uuid.UUID, int) (bool, error)
}

type VariantAvailableEvent struct {
	VariantID uuid.UUID
	EventID   uuid.UUID
	At        time.Time
}

func (VariantAvailableEvent) Topic() string               { return TopicVariantAvailable }
func (e VariantAvailableEvent) AggregateType() string     { return "inventory_variant" }
func (e VariantAvailableEvent) AggregateID() uuid.UUID    { return e.VariantID }
func (e VariantAvailableEvent) IdempotencyKey() uuid.UUID { return e.EventID }
func (e VariantAvailableEvent) OccurredAt() time.Time     { return e.At }
func (e VariantAvailableEvent) MarshalPayload() ([]byte, error) {
	if e.VariantID == uuid.Nil || e.EventID == uuid.Nil {
		return nil, errors.New("invalid variant available event")
	}
	return json.Marshal(struct {
		Version   int       `json:"version"`
		VariantID uuid.UUID `json:"variant_id"`
	}{1, e.VariantID})
}

var (
	ErrInsufficientStock      = errors.New("insufficient stock")
	ErrStockReadOnly          = errors.New("stock is read-only in external inventory mode")
	ErrReservationNotFound    = errors.New("inventory reservation not found")
	ErrReservationInactive    = errors.New("inventory reservation is not active")
	ErrExternalImportDisabled = errors.New("external stock import is disabled in internal inventory mode")
)

type Mode string

const (
	ModeInternal Mode = "internal"
	ModeExternal Mode = "external"
)

type ReservationRequest struct {
	IdempotencyKey uuid.UUID
	VariantID      uuid.UUID
	WarehouseID    uuid.UUID
	Quantity       int
	ExpiresAt      time.Time
}

type Reservation struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey"`
	IdempotencyKey uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null"`
	VariantID      uuid.UUID  `gorm:"type:uuid;not null"`
	WarehouseID    uuid.UUID  `gorm:"type:uuid;not null"`
	Quantity       int        `gorm:"not null"`
	Status         string     `gorm:"type:varchar(32);not null"`
	ExpiresAt      time.Time  `gorm:"not null"`
	OrderID        *uuid.UUID `gorm:"type:uuid"`
}

func (Reservation) TableName() string { return "inventory_reservations" }

// Repository must reserve atomically. Its Postgres adapter will perform the
// guarded update; no browser-supplied stock value participates in the decision.
type Repository interface {
	Reserve(context.Context, ReservationRequest) (*Reservation, error)
	ReserveBatch(context.Context, []ReservationRequest) ([]Reservation, error)
	Release(context.Context, uuid.UUID) error
	Commit(context.Context, uuid.UUID, uuid.UUID) error
	Adjust(context.Context, uuid.UUID, uuid.UUID, int) error
	ReplaceQuantity(context.Context, uuid.UUID, uuid.UUID, int) error
}

// ExternalStockImporter is intentionally narrower than Service. Sync is the
// only application boundary that receives authoritative ERP stock snapshots;
// browser/admin mutations continue to use Adjust and are blocked externally.
type ExternalStockImporter interface {
	ReplaceExternalQuantity(context.Context, uuid.UUID, uuid.UUID, int) error
}

// Service is the provider-neutral port consumed by Checkout and Orders.
type Service interface {
	Reserve(context.Context, ReservationRequest) (*Reservation, error)
	ReserveBatch(context.Context, []ReservationRequest) ([]Reservation, error)
	Release(context.Context, uuid.UUID) error
	Commit(context.Context, uuid.UUID, uuid.UUID) error
	Adjust(context.Context, uuid.UUID, uuid.UUID, int) error
}

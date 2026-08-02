package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrStockReadOnly     = errors.New("stock is read-only in external inventory mode")
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
	ID             uuid.UUID
	IdempotencyKey uuid.UUID
	VariantID      uuid.UUID
	WarehouseID    uuid.UUID
	Quantity       int
	Status         string
	ExpiresAt      time.Time
}

// Repository must reserve atomically. Its Postgres adapter will perform the
// guarded update; no browser-supplied stock value participates in the decision.
type Repository interface {
	Reserve(context.Context, ReservationRequest) (*Reservation, error)
	Release(context.Context, uuid.UUID) error
	Commit(context.Context, uuid.UUID, uuid.UUID) error
	Adjust(context.Context, uuid.UUID, uuid.UUID, int) error
}

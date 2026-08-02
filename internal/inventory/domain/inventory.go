package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrStockReadOnly       = errors.New("stock is read-only in external inventory mode")
	ErrReservationNotFound = errors.New("inventory reservation not found")
	ErrReservationInactive = errors.New("inventory reservation is not active")
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
	Release(context.Context, uuid.UUID) error
	Commit(context.Context, uuid.UUID, uuid.UUID) error
	Adjust(context.Context, uuid.UUID, uuid.UUID, int) error
}

// Service is the provider-neutral port consumed by Checkout and Orders.
type Service interface {
	Reserve(context.Context, ReservationRequest) (*Reservation, error)
	Release(context.Context, uuid.UUID) error
	Commit(context.Context, uuid.UUID, uuid.UUID) error
	Adjust(context.Context, uuid.UUID, uuid.UUID, int) error
}

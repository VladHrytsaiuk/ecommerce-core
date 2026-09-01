package domain

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type Campaign struct {
	ID, CartID                            uuid.UUID
	CustomerID                            *uuid.UUID
	ContactEmail                          string
	Step                                  int
	Status                                string
	DueAt, LockedAt, CreatedAt, UpdatedAt time.Time
}
type CartState struct {
	IsActive, IsPaid, IsEmpty bool
	LastUpdatedAt             time.Time
}
type Contact struct {
	CustomerID *uuid.UUID
	Email      string
}
type CartRecoveryReader interface {
	GetCartState(context.Context, uuid.UUID) (CartState, error)
}
type MarketingConsentReader interface {
	HasConsent(context.Context, *uuid.UUID, string) (bool, error)
}
type CartRecoveryContactProvider interface {
	ContactForCart(context.Context, uuid.UUID) (*Contact, error)
}
type Repository interface {
	ClaimDue(context.Context, time.Time) (*Campaign, error)
	Update(context.Context, *Campaign) error
	Create(context.Context, Campaign) error
	CreateOrReset(context.Context, Campaign) error
	Requeue(context.Context, uuid.UUID) error
}
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

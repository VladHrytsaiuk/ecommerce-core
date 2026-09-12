package domain

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type Campaign struct {
	ID, CartID   uuid.UUID
	CustomerID   *uuid.UUID
	ContactEmail string
	Step         int
	Status       string
	// LockToken identifies the claim that produced this campaign. Update and
	// Requeue present it, so a worker whose lease was taken over while it was
	// still running cannot write its outcome over the newer claim's.
	LockToken                             uuid.UUID
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
	// Requeue releases a claim whose pass failed. The token is the one the
	// claim handed out, so a worker whose lease was taken over cannot release
	// a claim another worker now holds.
	Requeue(ctx context.Context, campaignID, lockToken uuid.UUID) error
}
type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

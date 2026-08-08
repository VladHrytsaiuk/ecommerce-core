// Package domain defines the transport- and provider-neutral Comparison module.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidOwner      = errors.New("comparison owner is invalid")
	ErrInvalidVariant    = errors.New("comparison product variant is invalid")
	ErrVariantNotFound   = errors.New("comparison product variant is unavailable")
	ErrComparisonAtLimit = errors.New("comparison item limit reached")
)

// Owner identifies exactly one stable owner: the authenticated user or the
// server-issued anonymous browser session.
type Owner struct {
	UserID    *uuid.UUID
	SessionID *uuid.UUID
}

func (owner Owner) Valid() bool {
	return (owner.UserID != nil && *owner.UserID != uuid.Nil) != (owner.SessionID != nil && *owner.SessionID != uuid.Nil)
}

type Item struct {
	ID               uuid.UUID
	ProductVariantID uuid.UUID
	CreatedAt        time.Time
}

// List groups comparable variants by their stable Catalog category identity.
type List struct {
	ID         uuid.UUID
	CategoryID uuid.UUID
	Items      []Item
}

type Comparison struct {
	Owner Owner
	Lists []List
}

type ComparisonService interface {
	List(context.Context, Owner) (*Comparison, error)
	Add(context.Context, Owner, uuid.UUID) error
	Remove(context.Context, Owner, uuid.UUID) error
	MergeGuestComparison(context.Context, uuid.UUID, uuid.UUID) error
}

type ComparisonRepository interface {
	List(context.Context, Owner) ([]List, error)
	Add(context.Context, Owner, uuid.UUID, int) error
	Remove(context.Context, Owner, uuid.UUID) error
	MergeGuestComparison(context.Context, uuid.UUID, uuid.UUID, int) error
}

type Service = ComparisonService
type Repository = ComparisonRepository

//go:build !legacy

// Package domain defines the clean, provider-neutral shopping cart boundary.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidOwner = errors.New("cart owner is invalid")
	ErrInvalidItem  = errors.New("cart item is invalid")
	ErrItemNotFound = errors.New("cart item not found")
)

type Owner struct {
	CustomerID *uuid.UUID
	SessionID  *uuid.UUID
}

type Item struct {
	VariantID uuid.UUID
	Quantity  int
}

type Cart struct {
	ID               uuid.UUID
	Owner            Owner
	Status           string
	Items            []Item
	AppliedPromoCode string
	DiscountAmount   int64
	UpdatedAt        time.Time
}

type Repository interface {
	GetOrCreate(context.Context, Owner) (*Cart, error)
	Add(context.Context, Owner, Item) (*Cart, error)
	SetQuantity(context.Context, Owner, Item) (*Cart, error)
	Remove(context.Context, Owner, uuid.UUID) (*Cart, error)
	SetPromoCode(context.Context, Owner, string) (*Cart, error)
}

type Service interface {
	GetOrCreate(context.Context, Owner) (*Cart, error)
	Add(context.Context, Owner, Item) (*Cart, error)
	SetQuantity(context.Context, Owner, Item) (*Cart, error)
	Remove(context.Context, Owner, uuid.UUID) (*Cart, error)
	SetPromoCode(context.Context, Owner, string) (*Cart, error)
}

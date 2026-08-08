// Package domain defines the provider- and transport-neutral Wishlist module.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidOwner   = errors.New("wishlist owner is invalid")
	ErrInvalidVariant = errors.New("wishlist product variant is invalid")
)

// Owner identifies either an authenticated customer or an anonymous browser
// session. Exactly one value must be present.
type Owner struct {
	UserID    *uuid.UUID
	SessionID *uuid.UUID
}

func (owner Owner) Valid() bool {
	return (owner.UserID != nil && *owner.UserID != uuid.Nil) != (owner.SessionID != nil && *owner.SessionID != uuid.Nil)
}

type WishlistItem struct {
	ID               uuid.UUID
	ProductVariantID uuid.UUID
	CreatedAt        time.Time
}

type Wishlist struct {
	Owner Owner
	Items []WishlistItem
}

type WishlistService interface {
	List(context.Context, Owner) (*Wishlist, error)
	Add(context.Context, Owner, uuid.UUID) error
	Remove(context.Context, Owner, uuid.UUID) error
	MergeGuestWishlist(context.Context, uuid.UUID, uuid.UUID) error
}

type WishlistRepository interface {
	List(context.Context, Owner) ([]WishlistItem, error)
	Add(context.Context, Owner, uuid.UUID) error
	Remove(context.Context, Owner, uuid.UUID) error
	MergeGuestWishlist(context.Context, uuid.UUID, uuid.UUID) error
}

// Short aliases keep module call sites consistent with other clean modules.
type Service = WishlistService
type Repository = WishlistRepository

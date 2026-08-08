// Package service contains Wishlist use cases and depends only on its domain
// repository port.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

type Service struct{ repository domain.Repository }

func New(repository domain.Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, owner domain.Owner) (*domain.Wishlist, error) {
	if s.repository == nil || !owner.Valid() {
		return nil, domain.ErrInvalidOwner
	}
	items, err := s.repository.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	return &domain.Wishlist{Owner: owner, Items: items}, nil
}

func (s *Service) Add(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if s.repository == nil || !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	if variantID == uuid.Nil {
		return domain.ErrInvalidVariant
	}
	return s.repository.Add(ctx, owner, variantID)
}

func (s *Service) Remove(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if s.repository == nil || !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	if variantID == uuid.Nil {
		return domain.ErrInvalidVariant
	}
	return s.repository.Remove(ctx, owner, variantID)
}

func (s *Service) MergeGuestWishlist(ctx context.Context, userID, sessionID uuid.UUID) error {
	if s.repository == nil || userID == uuid.Nil || sessionID == uuid.Nil {
		return domain.ErrInvalidOwner
	}
	return s.repository.MergeGuestWishlist(ctx, userID, sessionID)
}

var _ domain.Service = (*Service)(nil)

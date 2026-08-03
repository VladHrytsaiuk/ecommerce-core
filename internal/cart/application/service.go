// Package application validates cart use cases without importing HTTP or GORM.
package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
)

type Service struct{ repo domain.Repository }

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

func (s *Service) GetOrCreate(ctx context.Context, owner domain.Owner) (*domain.Cart, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	return s.repo.GetOrCreate(ctx, owner)
}
func (s *Service) Add(ctx context.Context, owner domain.Owner, item domain.Item) (*domain.Cart, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	if err := validateItem(item); err != nil {
		return nil, err
	}
	return s.repo.Add(ctx, owner, item)
}
func (s *Service) SetQuantity(ctx context.Context, owner domain.Owner, item domain.Item) (*domain.Cart, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	if err := validateItem(item); err != nil {
		return nil, err
	}
	return s.repo.SetQuantity(ctx, owner, item)
}
func (s *Service) Remove(ctx context.Context, owner domain.Owner, variantID uuid.UUID) (*domain.Cart, error) {
	if err := validateOwner(owner); err != nil {
		return nil, err
	}
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("%w: variant id is required", domain.ErrInvalidItem)
	}
	return s.repo.Remove(ctx, owner, variantID)
}
func validateOwner(owner domain.Owner) error {
	if (owner.CustomerID == nil && owner.SessionID == nil) || (owner.CustomerID != nil && owner.SessionID != nil) {
		return domain.ErrInvalidOwner
	}
	return nil
}
func validateItem(item domain.Item) error {
	if item.VariantID == uuid.Nil || item.Quantity <= 0 {
		return domain.ErrInvalidItem
	}
	return nil
}

var _ domain.Service = (*Service)(nil)

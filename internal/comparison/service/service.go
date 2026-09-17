// Package service contains Comparison use cases and its configurable limit.
package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
)

type Service struct {
	repository domain.Repository
	maxItems   int
}

func New(repository domain.Repository, maxItems int) *Service {
	return &Service{repository: repository, maxItems: maxItems}
}

func (service *Service) List(ctx context.Context, owner domain.Owner) (*domain.Comparison, error) {
	if service.repository == nil || !owner.Valid() {
		return nil, domain.ErrInvalidOwner
	}
	lists, err := service.repository.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	return &domain.Comparison{Owner: owner, Lists: lists}, nil
}

func (service *Service) Add(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if service.repository == nil || !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	if variantID == uuid.Nil {
		return domain.ErrInvalidVariant
	}
	if service.maxItems <= 0 {
		return domain.ErrComparisonAtLimit
	}
	return service.repository.Add(ctx, owner, variantID, service.maxItems)
}

func (service *Service) Remove(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if service.repository == nil || !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	if variantID == uuid.Nil {
		return domain.ErrInvalidVariant
	}
	return service.repository.Remove(ctx, owner, variantID)
}

func (service *Service) MergeGuestComparison(ctx context.Context, userID, sessionID uuid.UUID) error {
	if service.repository == nil || userID == uuid.Nil || sessionID == uuid.Nil {
		return domain.ErrInvalidOwner
	}
	if service.maxItems <= 0 {
		return domain.ErrComparisonAtLimit
	}
	return service.repository.MergeGuestComparison(ctx, userID, sessionID, service.maxItems)
}

var _ domain.Service = (*Service)(nil)

// Package service contains Reviews application rules.
package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

const maxCommentRunes = 5000

type Service struct{ repository domain.Repository }

func New(repository domain.Repository) *Service { return &Service{repository: repository} }

func (service *Service) Create(ctx context.Context, command domain.CreateCommand) (*domain.Review, error) {
	if service.repository == nil || command.ProductID == uuid.Nil || command.UserID == uuid.Nil || command.Rating < 1 || command.Rating > 5 {
		return nil, domain.ErrInvalidReview
	}
	command.Comment = strings.TrimSpace(command.Comment)
	if command.Comment == "" || len([]rune(command.Comment)) > maxCommentRunes {
		return nil, domain.ErrInvalidReview
	}
	return service.repository.Create(ctx, command)
}

func (service *Service) ListApproved(ctx context.Context, productID uuid.UUID) ([]domain.Review, error) {
	if service.repository == nil || productID == uuid.Nil {
		return nil, domain.ErrInvalidReview
	}
	return service.repository.ListApproved(ctx, productID)
}

func (service *Service) SetStatus(ctx context.Context, reviewID uuid.UUID, status domain.Status) (*domain.Review, error) {
	if service.repository == nil || reviewID == uuid.Nil || !status.Valid() {
		return nil, domain.ErrInvalidReview
	}
	return service.repository.SetStatus(ctx, reviewID, status)
}

func (service *Service) Delete(ctx context.Context, reviewID uuid.UUID) error {
	if service.repository == nil || reviewID == uuid.Nil {
		return domain.ErrInvalidReview
	}
	return service.repository.Delete(ctx, reviewID)
}

var _ domain.Service = (*Service)(nil)

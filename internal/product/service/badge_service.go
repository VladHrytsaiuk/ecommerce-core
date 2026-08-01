package service

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

type badgeService struct {
	repo domain.BadgeRepository
	l    logger.Logger
}

func NewBadgeService(repo domain.BadgeRepository, l logger.Logger) domain.BadgeService {
	return &badgeService{
		repo: repo,
		l:    l,
	}
}

func (s *badgeService) GetAll(ctx context.Context) ([]domain.Badge, error) {
	return s.repo.FindAll(ctx)
}

func (s *badgeService) GetByID(ctx context.Context, id int) (*domain.Badge, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *badgeService) Create(ctx context.Context, badge *domain.Badge) error {
	return s.repo.Create(ctx, badge)
}

func (s *badgeService) Update(ctx context.Context, badge *domain.Badge) error {
	return s.repo.Update(ctx, badge)
}

func (s *badgeService) Delete(ctx context.Context, id int) error {
	count, err := s.repo.CountUsage(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return domain.ErrBadgeInUse
	}
	return s.repo.Delete(ctx, id)
}

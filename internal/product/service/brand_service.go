package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	slugLib "github.com/gosimple/slug"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

type brandService struct {
	repo        domain.BrandRepository
	productRepo domain.ProductRepository
	l           logger.Logger
}

func NewBrandService(repo domain.BrandRepository, productRepo domain.ProductRepository, l logger.Logger) domain.BrandService {
	return &brandService{
		repo:        repo,
		productRepo: productRepo,
		l:           l,
	}
}

func (s *brandService) GetAll(ctx context.Context) ([]domain.Brand, error) {
	return s.repo.FindAll(ctx)
}

func (s *brandService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *brandService) GenerateSlug(ctx context.Context, name string) (string, error) {
	baseSlug := slugLib.Make(name)
	if baseSlug == "" {
		baseSlug = "brand"
	}

	candidate := baseSlug
	for attempts := 1; attempts <= 100; attempts++ {
		exists, err := s.repo.SlugExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}

		candidate = fmt.Sprintf("%s-%d", baseSlug, attempts)
	}

	return candidate, nil
}

func (s *brandService) Create(ctx context.Context, brand *domain.Brand) error {
	if brand.ID == uuid.Nil {
		brand.ID = uuid.New()
	}
	if brand.Slug == "" {
		slug, _ := s.GenerateSlug(ctx, brand.Name)
		brand.Slug = slug
	}
	return s.repo.Create(ctx, brand)
}

func (s *brandService) Update(ctx context.Context, brand *domain.Brand) error {
	if brand.Slug == "" {
		slug, _ := s.GenerateSlug(ctx, brand.Name)
		brand.Slug = slug
	}
	return s.repo.Update(ctx, brand)
}

func (s *brandService) Delete(ctx context.Context, id uuid.UUID) error {
	count, err := s.productRepo.CountByBrand(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return domain.ErrBrandInUse
	}
	return s.repo.Delete(ctx, id)
}

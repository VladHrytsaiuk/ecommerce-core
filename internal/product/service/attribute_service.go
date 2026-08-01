package service

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

type attributeService struct {
	repo domain.AttributeRepository
	l    logger.Logger
}

func NewAttributeService(repo domain.AttributeRepository, l logger.Logger) domain.AttributeService {
	return &attributeService{repo: repo, l: l}
}

func (s *attributeService) GetAll(ctx context.Context, lang string) ([]domain.Attribute, error) {
	return s.repo.FindAll(ctx, lang)
}

func (s *attributeService) GetByID(ctx context.Context, id int, lang string) (*domain.Attribute, error) {
	return s.repo.FindByID(ctx, id, lang)
}

func (s *attributeService) GetValues(ctx context.Context, attributeCode, lang string) ([]domain.AttrValueOption, error) {
	return s.repo.FindValues(ctx, attributeCode, lang)
}

func (s *attributeService) Create(ctx context.Context, attribute *domain.Attribute) error {
	s.applyTranslationFallback(attribute)
	return s.repo.Create(ctx, attribute)
}

func (s *attributeService) Update(ctx context.Context, attribute *domain.Attribute) error {
	s.applyTranslationFallback(attribute)
	return s.repo.Update(ctx, attribute)
}

func (s *attributeService) Delete(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}

func (s *attributeService) UpdateOrder(ctx context.Context, ids []int) error {
	return s.repo.UpdateOrder(ctx, ids)
}

func (s *attributeService) applyTranslationFallback(attribute *domain.Attribute) {
	var ukT, enT *domain.AttributeTranslation
	for i := range attribute.Translations {
		t := &attribute.Translations[i]
		if t.LanguageCode == "uk" {
			ukT = t
		}
		if t.LanguageCode == "en" {
			enT = t
		}
	}

	if ukT != nil && enT != nil {
		if enT.Name == "" {
			enT.Name = ukT.Name
		}
	}
}

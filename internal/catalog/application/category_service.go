package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

// CategoryService enforces locale and translation-list invariants for the
// clean Catalog category aggregate.
type CategoryService struct {
	repo    domain.CategoryRepository
	locales localePolicy
}

func NewCategoryService(repo domain.CategoryRepository, allowedLocales []string) *CategoryService {
	return &CategoryService{repo: repo, locales: newLocalePolicy(allowedLocales)}
}

func (s *CategoryService) FindBySlug(ctx context.Context, locale, slug string) (*domain.Category, error) {
	locale = normalize(locale)
	slug = strings.TrimSpace(slug)
	if locale == "" || slug == "" {
		return nil, fmt.Errorf("%w: locale and slug are required", domain.ErrInvalidCatalogCategory)
	}
	if !s.locales.allows(locale) {
		return nil, fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidCatalogCategory, locale)
	}
	return s.repo.FindBySlug(ctx, locale, slug)
}

func (s *CategoryService) Create(ctx context.Context, category *domain.Category) error {
	if err := s.validate(category); err != nil {
		return err
	}
	if category.ID == uuid.Nil {
		category.ID = uuid.New()
	}
	return s.repo.Create(ctx, category)
}

func (s *CategoryService) validate(category *domain.Category) error {
	if category == nil {
		return fmt.Errorf("%w: category is required", domain.ErrInvalidCatalogCategory)
	}
	if len(category.Translations) == 0 {
		return fmt.Errorf("%w: at least one translation is required", domain.ErrInvalidCatalogCategory)
	}
	seenLocales := make(map[string]struct{}, len(category.Translations))
	for i := range category.Translations {
		translation := &category.Translations[i]
		translation.Locale = normalize(translation.Locale)
		translation.Name = strings.TrimSpace(translation.Name)
		translation.Slug = strings.TrimSpace(translation.Slug)
		if translation.Locale == "" || translation.Name == "" || translation.Slug == "" {
			return fmt.Errorf("%w: translation locale, name and slug are required", domain.ErrInvalidCatalogCategory)
		}
		if !s.locales.allows(translation.Locale) {
			return fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidCatalogCategory, translation.Locale)
		}
		if _, exists := seenLocales[translation.Locale]; exists {
			return fmt.Errorf("%w: duplicate translation for locale %q", domain.ErrInvalidCatalogCategory, translation.Locale)
		}
		seenLocales[translation.Locale] = struct{}{}
	}
	return nil
}

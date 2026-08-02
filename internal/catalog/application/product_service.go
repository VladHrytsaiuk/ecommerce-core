// Package application contains provider- and delivery-neutral Catalog use cases.
package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

// ProductService validates Catalog invariants before delegating persistence to
// the repository port. It is intentionally independent of Gin and GORM.
type ProductService struct {
	repo           domain.ProductRepository
	allowedLocales map[string]struct{}
}

func NewProductService(repo domain.ProductRepository, allowedLocales []string) *ProductService {
	locales := make(map[string]struct{}, len(allowedLocales))
	for _, locale := range allowedLocales {
		if normalized := normalize(locale); normalized != "" {
			locales[normalized] = struct{}{}
		}
	}
	return &ProductService{repo: repo, allowedLocales: locales}
}

func (s *ProductService) FindBySlug(ctx context.Context, locale, slug string) (*domain.Product, error) {
	locale = normalize(locale)
	slug = strings.TrimSpace(slug)
	if locale == "" || slug == "" {
		return nil, fmt.Errorf("%w: locale and slug are required", domain.ErrInvalidProduct)
	}
	if !s.localeAllowed(locale) {
		return nil, fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidProduct, locale)
	}
	return s.repo.FindBySlug(ctx, locale, slug)
}

func (s *ProductService) Create(ctx context.Context, product *domain.Product) error {
	if err := s.validate(product); err != nil {
		return err
	}
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	return s.repo.Create(ctx, product)
}

func (s *ProductService) validate(product *domain.Product) error {
	if product == nil {
		return fmt.Errorf("%w: product is required", domain.ErrInvalidProduct)
	}
	product.Status = normalize(product.Status)
	if product.Status == "" {
		product.Status = "draft"
	}
	if product.Status != "draft" && product.Status != "active" && product.Status != "archived" {
		return fmt.Errorf("%w: unsupported product status %q", domain.ErrInvalidProduct, product.Status)
	}
	if len(product.Translations) == 0 {
		return fmt.Errorf("%w: at least one translation is required", domain.ErrInvalidProduct)
	}

	seenLocales := make(map[string]struct{}, len(product.Translations))
	for i := range product.Translations {
		translation := &product.Translations[i]
		translation.Locale = normalize(translation.Locale)
		translation.Name = strings.TrimSpace(translation.Name)
		translation.Slug = strings.TrimSpace(translation.Slug)
		if translation.Locale == "" || translation.Name == "" || translation.Slug == "" {
			return fmt.Errorf("%w: translation locale, name and slug are required", domain.ErrInvalidProduct)
		}
		if !s.localeAllowed(translation.Locale) {
			return fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidProduct, translation.Locale)
		}
		if _, exists := seenLocales[translation.Locale]; exists {
			return fmt.Errorf("%w: duplicate translation for locale %q", domain.ErrInvalidProduct, translation.Locale)
		}
		seenLocales[translation.Locale] = struct{}{}
	}
	return nil
}

func (s *ProductService) localeAllowed(locale string) bool {
	_, ok := s.allowedLocales[locale]
	return ok
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

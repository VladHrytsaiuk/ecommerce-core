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
	repo         domain.ProductRepository
	locales      localePolicy
	ratingReader domain.ProductRatingReader
	seoReader    domain.ProductSEOReader
	badgeReader  domain.ProductBadgeReader
}

func NewProductService(repo domain.ProductRepository, allowedLocales []string) *ProductService {
	return &ProductService{repo: repo, locales: newLocalePolicy(allowedLocales)}
}

func (s *ProductService) WithRatingReader(reader domain.ProductRatingReader) *ProductService {
	s.ratingReader = reader
	return s
}

func (s *ProductService) WithSEOReader(reader domain.ProductSEOReader) *ProductService {
	s.seoReader = reader
	return s
}

func (s *ProductService) WithBadgeReader(reader domain.ProductBadgeReader) *ProductService {
	s.badgeReader = reader
	return s
}

func (s *ProductService) FindBySlug(ctx context.Context, locale, slug string) (*domain.Product, error) {
	locale = normalize(locale)
	slug = strings.TrimSpace(slug)
	if locale == "" || slug == "" {
		return nil, fmt.Errorf("%w: locale and slug are required", domain.ErrInvalidProduct)
	}
	if !s.locales.allows(locale) {
		return nil, fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidProduct, locale)
	}
	product, err := s.repo.FindBySlug(ctx, locale, slug)
	if err != nil || product == nil {
		return product, err
	}
	if err := s.enrich(ctx, []*domain.Product{product}, locale); err != nil {
		return nil, err
	}
	return product, nil
}

// List enriches a catalog page with at most one bulk read per optional module.
// It never invokes an optional reader once per product.
func (s *ProductService) List(ctx context.Context, locale string) ([]domain.Product, error) {
	locale = normalize(locale)
	if locale == "" || !s.locales.allows(locale) {
		return nil, fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidProduct, locale)
	}
	products, err := s.repo.List(ctx)
	if err != nil || len(products) == 0 {
		return products, err
	}
	pointers := make([]*domain.Product, 0, len(products))
	for index := range products {
		pointers = append(pointers, &products[index])
	}
	if err := s.enrich(ctx, pointers, locale); err != nil {
		return nil, err
	}
	return products, nil
}

func (s *ProductService) enrich(ctx context.Context, products []*domain.Product, locale string) error {
	ids := make([]uuid.UUID, 0, len(products))
	for _, product := range products {
		if product != nil && product.ID != uuid.Nil {
			ids = append(ids, product.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	if s.ratingReader != nil {
		ratings, err := s.ratingReader.RatingsForProducts(ctx, ids)
		if err != nil {
			return err
		}
		for _, product := range products {
			if value, ok := ratings[product.ID]; ok {
				product.Rating = &value
			}
		}
	}
	if s.seoReader != nil {
		metadata, err := s.seoReader.SEOForResources(ctx, "product", ids, locale)
		if err != nil {
			return err
		}
		for _, product := range products {
			if value, ok := metadata[product.ID]; ok {
				product.SEO = &value
			}
		}
	}
	if s.badgeReader != nil {
		badges, err := s.badgeReader.BadgesForProducts(ctx, ids, locale)
		if err != nil {
			return err
		}
		for _, product := range products {
			if values, ok := badges[product.ID]; ok {
				product.Badges = values
			}
		}
	}
	return nil
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

func (s *ProductService) Update(ctx context.Context, product *domain.Product) error {
	if product == nil || product.ID == uuid.Nil {
		return fmt.Errorf("%w: product id is required", domain.ErrInvalidProduct)
	}
	if err := s.validate(product); err != nil {
		return err
	}
	repository, ok := s.repo.(domain.AdminProductRepository)
	if !ok {
		return fmt.Errorf("catalog product update is not configured")
	}
	return repository.Update(ctx, product)
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
		if !s.locales.allows(translation.Locale) {
			return fmt.Errorf("%w: locale %q is not enabled for this store", domain.ErrInvalidProduct, translation.Locale)
		}
		if _, exists := seenLocales[translation.Locale]; exists {
			return fmt.Errorf("%w: duplicate translation for locale %q", domain.ErrInvalidProduct, translation.Locale)
		}
		seenLocales[translation.Locale] = struct{}{}
	}
	return nil
}

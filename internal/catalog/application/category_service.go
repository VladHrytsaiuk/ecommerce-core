package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

const (
	categoryCachePrefix = "catalog:category:v1:"
	categoryCacheTTL    = 10 * time.Minute
)

// CategoryService enforces locale and translation-list invariants for the
// clean Catalog category aggregate.
type CategoryService struct {
	repo    domain.CategoryRepository
	locales localePolicy
	cache   cache.Service
}

func NewCategoryService(repo domain.CategoryRepository, allowedLocales []string) *CategoryService {
	return &CategoryService{repo: repo, locales: newLocalePolicy(allowedLocales)}
}

// WithCache attaches an optional provider-neutral cache from Bootstrap.
func (s *CategoryService) WithCache(service cache.Service) *CategoryService {
	s.cache = service
	return s
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
	key := categoryCacheKey(locale, slug)
	if s.cache != nil {
		if encoded, err := s.cache.Get(ctx, key); err == nil {
			var category domain.Category
			if err := json.Unmarshal(encoded, &category); err == nil {
				return &category, nil
			}
			_ = s.cache.Delete(ctx, key)
		} else if err != cache.ErrMiss {
			// Caching is an optimization; a transient cache failure must not turn a
			// catalog read into an outage.
		}
	}
	category, err := s.repo.FindBySlug(ctx, locale, slug)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		if encoded, marshalErr := json.Marshal(category); marshalErr == nil {
			_ = s.cache.Set(ctx, key, encoded, categoryCacheTTL)
		}
	}
	return category, nil
}

func (s *CategoryService) Create(ctx context.Context, category *domain.Category) error {
	if err := s.validate(category); err != nil {
		return err
	}
	if category.ID == uuid.Nil {
		category.ID = uuid.New()
	}
	if err := s.repo.Create(ctx, category); err != nil {
		return err
	}
	if s.cache != nil {
		// A category can add translations/paths visible to multiple read keys.
		// Prefix invalidation is correct and cheap with the Redis SCAN adapter.
		_ = s.cache.DeleteByPrefix(ctx, categoryCachePrefix)
	}
	return nil
}

func categoryCacheKey(locale, slug string) string { return categoryCachePrefix + locale + ":" + slug }

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

package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

func TestCategoryServiceCreatePersistsThreeTranslations(t *testing.T) {
	repo := &fakeCategoryRepository{}
	service := NewCategoryService(repo, []string{"es", "en", "ca"})
	category := &domain.Category{Translations: []domain.CategoryTranslation{
		{Locale: "es", Name: " Cuidado facial ", Slug: "cuidado-facial"},
		{Locale: "en", Name: "Skin care", Slug: "skin-care"},
		{Locale: "ca", Name: "Cura facial", Slug: "cura-facial"},
	}}

	if err := service.Create(context.Background(), category); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repo.created || category.Translations[0].Locale != "es" || category.Translations[0].Name != "Cuidado facial" {
		t.Fatalf("category was not normalized and persisted: %+v", category)
	}
}

func TestCategoryServiceCachesFindAndInvalidatesOnCreate(t *testing.T) {
	repo := &fakeCategoryRepository{found: &domain.Category{Translations: []domain.CategoryTranslation{{Locale: "en", Name: "Skin", Slug: "skin"}}}}
	cacheService := &memoryCache{values: make(map[string][]byte)}
	service := NewCategoryService(repo, []string{"en"}).WithCache(cacheService)

	if _, err := service.FindBySlug(context.Background(), "en", "skin"); err != nil {
		t.Fatalf("first FindBySlug() error = %v", err)
	}
	if _, err := service.FindBySlug(context.Background(), "en", "skin"); err != nil {
		t.Fatalf("second FindBySlug() error = %v", err)
	}
	if repo.finds != 1 {
		t.Fatalf("repository calls = %d, want 1 after cache hit", repo.finds)
	}
	if err := service.Create(context.Background(), &domain.Category{Translations: []domain.CategoryTranslation{{Locale: "en", Name: "Hair", Slug: "hair"}}}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(cacheService.values) != 0 {
		t.Fatalf("cache entries = %d, want 0 after invalidation", len(cacheService.values))
	}
}

func TestCategoryServiceCreateRejectsDuplicateLocale(t *testing.T) {
	service := NewCategoryService(&fakeCategoryRepository{}, []string{"es", "en"})
	err := service.Create(context.Background(), &domain.Category{Translations: []domain.CategoryTranslation{
		{Locale: "es", Name: "Cuidado", Slug: "cuidado"},
		{Locale: "ES", Name: "Skin care", Slug: "skin-care"},
	}})
	if !errors.Is(err, domain.ErrInvalidCatalogCategory) {
		t.Fatalf("Create() error = %v, want ErrInvalidCatalogCategory", err)
	}
}

type fakeCategoryRepository struct {
	created bool
	found   *domain.Category
	finds   int
}

func (r *fakeCategoryRepository) FindBySlug(_ context.Context, _, _ string) (*domain.Category, error) {
	r.finds++
	if r.found != nil {
		return r.found, nil
	}
	return nil, domain.ErrCatalogCategoryNotFound
}

type memoryCache struct{ values map[string][]byte }

func (c *memoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.values[key] = append([]byte(nil), value...)
	return nil
}
func (c *memoryCache) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := c.values[key]
	if !ok {
		return nil, cache.ErrMiss
	}
	return append([]byte(nil), value...), nil
}
func (c *memoryCache) Delete(_ context.Context, key string) error { delete(c.values, key); return nil }
func (c *memoryCache) DeleteByPrefix(_ context.Context, prefix string) error {
	for key := range c.values {
		if strings.HasPrefix(key, prefix) {
			delete(c.values, key)
		}
	}
	return nil
}

var _ cache.Service = (*memoryCache)(nil)

func (r *fakeCategoryRepository) Create(_ context.Context, _ *domain.Category) error {
	r.created = true
	return nil
}

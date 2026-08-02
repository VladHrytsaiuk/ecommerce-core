package application

import (
	"context"
	"errors"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
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
}

func (r *fakeCategoryRepository) FindBySlug(_ context.Context, _, _ string) (*domain.Category, error) {
	return nil, domain.ErrCatalogCategoryNotFound
}

func (r *fakeCategoryRepository) Create(_ context.Context, _ *domain.Category) error {
	r.created = true
	return nil
}

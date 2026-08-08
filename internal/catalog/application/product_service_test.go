package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

func TestProductServiceCreateNormalizesThreeTranslations(t *testing.T) {
	repo := &fakeProductRepository{}
	service := NewProductService(repo, []string{"es", "en", "ca"})
	product := &domain.Product{
		Status: " ACTIVE ",
		Translations: []domain.ProductTranslation{
			{Locale: "ES", Name: " Crema ", Slug: "crema"},
			{Locale: "en", Name: "Cream", Slug: "cream"},
			{Locale: "ca", Name: "Crema", Slug: "crema-ca"},
		},
	}

	if err := service.Create(context.Background(), product); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repo.created || product.ID == uuid.Nil || product.Status != "active" {
		t.Fatalf("product was not normalized and persisted: %+v", product)
	}
	if product.Translations[0].Locale != "es" || product.Translations[0].Name != "Crema" {
		t.Fatalf("translation was not normalized: %+v", product.Translations[0])
	}
}

func TestProductServiceReadsOptionalRatingProjection(t *testing.T) {
	productID := uuid.New()
	repository := &fakeProductRepository{product: &domain.Product{ID: productID}}
	service := NewProductService(repository, []string{"en"}).WithRatingReader(ratingReaderFake{rating: &domain.ProductRating{ReviewCount: 2, AverageHundredths: 450}})

	product, err := service.FindBySlug(context.Background(), "en", "product")
	if err != nil || product.Rating == nil || product.Rating.ReviewCount != 2 || product.Rating.AverageHundredths != 450 {
		t.Fatalf("FindBySlug() = (%+v, %v), want rating projection", product, err)
	}
}

type ratingReaderFake struct{ rating *domain.ProductRating }

func (reader ratingReaderFake) RatingForProduct(context.Context, uuid.UUID) (*domain.ProductRating, error) {
	return reader.rating, nil
}

func TestProductServiceCreateRejectsDuplicateLocale(t *testing.T) {
	service := NewProductService(&fakeProductRepository{}, []string{"es", "en"})
	err := service.Create(context.Background(), &domain.Product{Translations: []domain.ProductTranslation{
		{Locale: "es", Name: "Crema", Slug: "crema"},
		{Locale: "ES", Name: "Cream", Slug: "cream"},
	}})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

func TestProductServiceCreateRejectsDisabledLocale(t *testing.T) {
	service := NewProductService(&fakeProductRepository{}, []string{"es", "en"})
	err := service.Create(context.Background(), &domain.Product{Translations: []domain.ProductTranslation{
		{Locale: "ca", Name: "Crema", Slug: "crema"},
	}})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

func TestProductServiceCreateRejectsUnsupportedStatus(t *testing.T) {
	service := NewProductService(&fakeProductRepository{}, []string{"es"})
	err := service.Create(context.Background(), &domain.Product{Status: "deleted", Translations: []domain.ProductTranslation{{Locale: "es", Name: "Crema", Slug: "crema"}}})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

type fakeProductRepository struct {
	created bool
	product *domain.Product
}

func (r *fakeProductRepository) FindBySlug(_ context.Context, _, _ string) (*domain.Product, error) {
	return r.product, nil
}

func (r *fakeProductRepository) Create(_ context.Context, product *domain.Product) error {
	r.created = true
	r.product = product
	return nil
}

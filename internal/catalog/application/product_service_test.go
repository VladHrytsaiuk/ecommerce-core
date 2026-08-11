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

func TestProductServiceListUsesOneBulkLookupPerOptionalModule(t *testing.T) {
	products := []domain.Product{{ID: uuid.New()}, {ID: uuid.New()}, {ID: uuid.New()}}
	repository := &listProductRepository{products: products}
	seo := &seoReaderFake{}
	badges := &badgeReaderFake{}
	service := NewProductService(repository, []string{"en"}).WithSEOReader(seo).WithBadgeReader(badges)

	result, err := service.List(context.Background(), "en")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if seo.calls != 1 || badges.calls != 1 {
		t.Fatalf("bulk readers called seo=%d badges=%d, want one each", seo.calls, badges.calls)
	}
	if len(result) != 3 || result[0].SEO == nil || len(result[0].Badges) != 1 {
		t.Fatalf("List() did not enrich products: %+v", result)
	}
}

type ratingReaderFake struct{ rating *domain.ProductRating }

func (reader ratingReaderFake) RatingForProduct(context.Context, uuid.UUID) (*domain.ProductRating, error) {
	return reader.rating, nil
}

func (reader ratingReaderFake) RatingsForProducts(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.ProductRating, error) {
	result := make(map[uuid.UUID]domain.ProductRating, len(ids))
	for _, id := range ids {
		if reader.rating != nil {
			result[id] = *reader.rating
		}
	}
	return result, nil
}

type seoReaderFake struct{ calls int }

func (reader *seoReaderFake) SEOForResources(_ context.Context, resourceType string, ids []uuid.UUID, locale string) (map[uuid.UUID]domain.ProductSEO, error) {
	reader.calls++
	if resourceType != "product" || locale != "en" {
		return nil, errors.New("unexpected SEO bulk query")
	}
	result := make(map[uuid.UUID]domain.ProductSEO, len(ids))
	for _, id := range ids {
		result[id] = domain.ProductSEO{Title: "SEO"}
	}
	return result, nil
}

type badgeReaderFake struct{ calls int }

func (reader *badgeReaderFake) BadgesForProducts(_ context.Context, ids []uuid.UUID, locale string) (map[uuid.UUID][]domain.ProductBadge, error) {
	reader.calls++
	if locale != "en" {
		return nil, errors.New("unexpected badge bulk query")
	}
	result := make(map[uuid.UUID][]domain.ProductBadge, len(ids))
	for _, id := range ids {
		result[id] = []domain.ProductBadge{{ID: uuid.New(), Slug: "new"}}
	}
	return result, nil
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

func (r *fakeProductRepository) List(_ context.Context) ([]domain.Product, error) {
	if r.product == nil {
		return nil, nil
	}
	return []domain.Product{*r.product}, nil
}
func (r *fakeProductRepository) ListProducts(_ context.Context, _ string, _, _ int) ([]domain.Product, int64, error) {
	products, err := r.List(context.Background())
	return products, int64(len(products)), err
}

type listProductRepository struct{ products []domain.Product }

func (r *listProductRepository) FindBySlug(context.Context, string, string) (*domain.Product, error) {
	return nil, domain.ErrProductNotFound
}
func (r *listProductRepository) List(context.Context) ([]domain.Product, error) {
	return r.products, nil
}
func (r *listProductRepository) ListProducts(context.Context, string, int, int) ([]domain.Product, int64, error) {
	return r.products, int64(len(r.products)), nil
}
func (r *listProductRepository) Create(context.Context, *domain.Product) error { return nil }

func (r *fakeProductRepository) FindBySlug(_ context.Context, _, _ string) (*domain.Product, error) {
	return r.product, nil
}

func (r *fakeProductRepository) Create(_ context.Context, product *domain.Product) error {
	r.created = true
	r.product = product
	return nil
}

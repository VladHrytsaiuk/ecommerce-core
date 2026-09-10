package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

func TestVariantServiceCreatesConfiguredCurrencyVariant(t *testing.T) {
	repo := &fakeVariantRepository{}
	service := NewVariantService(repo, []string{"es", "en"}, "EUR")
	price := mustMoney(1299, "eur")
	variant := &domain.ProductVariant{ProductID: uuid.New(), SKU: " CREAM-50 ", Price: price}

	if err := service.Create(context.Background(), variant); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repo.created || variant.ID == uuid.Nil || variant.SKU != "CREAM-50" || variant.Price.Currency() != "EUR" {
		t.Fatalf("variant was not normalized and persisted: %+v", variant)
	}
}

func TestVariantServiceRejectsDifferentCurrency(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"es"}, "EUR")
	price := mustMoney(100, "UAH")
	err := service.Create(context.Background(), &domain.ProductVariant{ProductID: uuid.New(), Price: price})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

func TestVariantServiceCheckoutLookupRequiresEnabledLocale(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"es"}, "EUR")
	_, err := service.FindActiveForCheckout(context.Background(), uuid.New(), "en")
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("FindActiveForCheckout() error = %v, want ErrInvalidProduct", err)
	}
}

func TestVariantServiceRejectsUnsupportedStatus(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"es"}, "EUR")
	price := mustMoney(100, "EUR")
	err := service.Create(context.Background(), &domain.ProductVariant{ProductID: uuid.New(), SKU: "CREAM-50", Status: "deleted", Price: price})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

func TestVariantServiceRejectsNegativeWeight(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"es"}, "EUR")
	price := mustMoney(100, "EUR")
	err := service.Create(context.Background(), &domain.ProductVariant{ProductID: uuid.New(), Price: price, WeightGrams: -1})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

func TestVariantServiceRejectsDuplicateOrForeignOptionValues(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"es"}, "EUR")
	productID := uuid.New()
	optionID := uuid.New()
	variant := &domain.ProductVariant{
		ProductID: productID,
		Price:     mustMoney(100, "EUR"),
		OptionValues: []domain.ProductOptionValue{
			{ID: uuid.New(), OptionID: optionID, ProductID: productID, Value: "Red"},
			{ID: uuid.New(), OptionID: optionID, ProductID: productID, Value: "Blue"},
		},
	}
	if err := service.Create(context.Background(), variant); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}

	variant.OptionValues = []domain.ProductOptionValue{{ID: uuid.New(), OptionID: uuid.New(), ProductID: uuid.New(), Value: "Red"}}
	if err := service.Create(context.Background(), variant); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Create() error = %v, want ErrInvalidProduct", err)
	}
}

type fakeVariantRepository struct {
	created       bool
	askedLocale   string
	askedFallback string
}

func (r *fakeVariantRepository) CreateVariant(_ context.Context, _ *domain.ProductVariant) error {
	r.created = true
	return nil
}

func (r *fakeVariantRepository) FindActiveForCheckout(_ context.Context, variantID uuid.UUID, locale, fallbackLocale string) (*domain.CheckoutVariant, error) {
	r.askedLocale, r.askedFallback = locale, fallbackLocale
	return &domain.CheckoutVariant{VariantID: variantID}, nil
}

func TestFindActiveForCheckoutPassesConfiguredFallbackLocale(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"uk", "en"}, "UAH").WithFallbackLocale("UK")

	if _, err := service.FindActiveForCheckout(context.Background(), uuid.New(), "en"); err != nil {
		t.Fatalf("FindActiveForCheckout() error = %v", err)
	}
	// Without the fallback a product awaiting its English translation was
	// reported as not found, which failed the whole checkout.
	if repository.askedLocale != "en" || repository.askedFallback != "uk" {
		t.Fatalf("lookup locales = %q/%q, want en/uk", repository.askedLocale, repository.askedFallback)
	}
}

func TestFindActiveForCheckoutStaysStrictWithoutConfiguredFallback(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"uk", "en"}, "UAH")

	if _, err := service.FindActiveForCheckout(context.Background(), uuid.New(), "en"); err != nil {
		t.Fatalf("FindActiveForCheckout() error = %v", err)
	}
	if repository.askedLocale != "en" || repository.askedFallback != "en" {
		t.Fatalf("lookup locales = %q/%q, want the requested locale for both", repository.askedLocale, repository.askedFallback)
	}
}

func TestWithFallbackLocaleRejectsUnsupportedLocale(t *testing.T) {
	repository := &fakeVariantRepository{}
	// A fallback outside SUPPORTED_LOCALES would silently surface text from a
	// locale the store never enabled.
	service := NewVariantService(repository, []string{"uk", "en"}, "UAH").WithFallbackLocale("de")

	if _, err := service.FindActiveForCheckout(context.Background(), uuid.New(), "en"); err != nil {
		t.Fatalf("FindActiveForCheckout() error = %v", err)
	}
	if repository.askedFallback != "en" {
		t.Fatalf("fallback = %q, want the unsupported locale ignored", repository.askedFallback)
	}
}

package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

func TestVariantServiceCreatesConfiguredCurrencyVariant(t *testing.T) {
	repo := &fakeVariantRepository{}
	service := NewVariantService(repo, []string{"es", "en"}, "EUR")
	price, _ := money.New(1299, "eur")
	variant := &domain.ProductVariant{ProductID: uuid.New(), SKU: " CREAM-50 ", Price: price}

	if err := service.Create(context.Background(), variant); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !repo.created || variant.ID == uuid.Nil || variant.SKU != "CREAM-50" || variant.Price.Currency != "EUR" {
		t.Fatalf("variant was not normalized and persisted: %+v", variant)
	}
}

func TestVariantServiceRejectsDifferentCurrency(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"es"}, "EUR")
	price, _ := money.New(100, "UAH")
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

type fakeVariantRepository struct {
	created bool
}

func (r *fakeVariantRepository) CreateVariant(_ context.Context, _ *domain.ProductVariant) error {
	r.created = true
	return nil
}

func (r *fakeVariantRepository) FindActiveForCheckout(_ context.Context, variantID uuid.UUID, _ string) (*domain.CheckoutVariant, error) {
	return &domain.CheckoutVariant{VariantID: variantID}, nil
}

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
	_, err := service.FindActiveForCheckoutBatch(context.Background(), []uuid.UUID{uuid.New()}, "en")
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("FindActiveForCheckoutBatch() error = %v, want ErrInvalidProduct", err)
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
	askedIDs      []uuid.UUID
	updated       domain.UpdateVariantCommand
	archived      uuid.UUID
}

func (r *fakeVariantRepository) CreateVariant(_ context.Context, _ *domain.ProductVariant) error {
	r.created = true
	return nil
}

func (r *fakeVariantRepository) FindVariantForUpdate(_ context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	return &domain.ProductVariant{ID: variantID, Status: "active"}, nil
}

func (r *fakeVariantRepository) UpdateVariant(_ context.Context, variantID uuid.UUID, command domain.UpdateVariantCommand) (*domain.ProductVariant, error) {
	r.updated = command
	return &domain.ProductVariant{ID: variantID, SKU: command.SKU, Status: command.Status, Price: command.Price, WeightGrams: command.WeightGrams}, nil
}

func (r *fakeVariantRepository) ArchiveVariant(_ context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	r.archived = variantID
	return &domain.ProductVariant{ID: variantID, Status: "archived"}, nil
}

func (r *fakeVariantRepository) FindActiveForCheckoutBatch(_ context.Context, variantIDs []uuid.UUID, locale, fallbackLocale string) (map[uuid.UUID]domain.CheckoutVariant, error) {
	r.askedLocale, r.askedFallback, r.askedIDs = locale, fallbackLocale, variantIDs
	found := make(map[uuid.UUID]domain.CheckoutVariant, len(variantIDs))
	for _, variantID := range variantIDs {
		found[variantID] = domain.CheckoutVariant{VariantID: variantID}
	}
	return found, nil
}

func TestFindActiveForCheckoutBatchPassesConfiguredFallbackLocale(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"uk", "en"}, "UAH").WithFallbackLocale("UK")

	if _, err := service.FindActiveForCheckoutBatch(context.Background(), []uuid.UUID{uuid.New()}, "en"); err != nil {
		t.Fatalf("FindActiveForCheckoutBatch() error = %v", err)
	}
	// Without the fallback a product awaiting its English translation was
	// reported as not found, which failed the whole checkout.
	if repository.askedLocale != "en" || repository.askedFallback != "uk" {
		t.Fatalf("lookup locales = %q/%q, want en/uk", repository.askedLocale, repository.askedFallback)
	}
}

func TestFindActiveForCheckoutBatchStaysStrictWithoutConfiguredFallback(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"uk", "en"}, "UAH")

	if _, err := service.FindActiveForCheckoutBatch(context.Background(), []uuid.UUID{uuid.New()}, "en"); err != nil {
		t.Fatalf("FindActiveForCheckoutBatch() error = %v", err)
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

	if _, err := service.FindActiveForCheckoutBatch(context.Background(), []uuid.UUID{uuid.New()}, "en"); err != nil {
		t.Fatalf("FindActiveForCheckoutBatch() error = %v", err)
	}
	if repository.askedFallback != "en" {
		t.Fatalf("fallback = %q, want the unsupported locale ignored", repository.askedFallback)
	}
}

func TestFindActiveForCheckoutBatchDeduplicatesVariantIDs(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"uk", "en"}, "UAH")
	shared := uuid.New()
	other := uuid.New()

	// A cart can list the same variant on several lines. Passing duplicates
	// through would widen the IN list for no benefit.
	if _, err := service.FindActiveForCheckoutBatch(context.Background(), []uuid.UUID{shared, other, shared}, "en"); err != nil {
		t.Fatalf("FindActiveForCheckoutBatch() error = %v", err)
	}
	if len(repository.askedIDs) != 2 {
		t.Fatalf("queried ids = %d, want 2 distinct variants", len(repository.askedIDs))
	}
}

func TestFindActiveForCheckoutBatchRejectsEmptyAndNilInput(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"uk", "en"}, "UAH")

	if _, err := service.FindActiveForCheckoutBatch(context.Background(), nil, "en"); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("empty batch error = %v, want ErrInvalidProduct", err)
	}
	if _, err := service.FindActiveForCheckoutBatch(context.Background(), []uuid.UUID{uuid.Nil}, "en"); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("nil variant id error = %v, want ErrInvalidProduct", err)
	}
}

func TestUpdateVariantEnforcesTheStoreCurrency(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"en"}, "EUR")

	// A price edited into another currency would be compared against order
	// totals and cart subtotals that are all denominated in the store's own.
	_, err := service.Update(context.Background(), uuid.New(), domain.UpdateVariantCommand{Price: mustMoney(100, "UAH")})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Update() error = %v, want ErrInvalidProduct", err)
	}
}

func TestUpdateVariantRejectsAnUnknownStatus(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"en"}, "EUR")

	// product_variants constrains status, so an unknown value would otherwise
	// fail inside the driver instead of as a clear validation error.
	_, err := service.Update(context.Background(), uuid.New(), domain.UpdateVariantCommand{Status: "discontinued", Price: mustMoney(100, "EUR")})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Update() error = %v, want ErrInvalidProduct", err)
	}
}

func TestUpdateVariantNormalizesItsInput(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"en"}, "EUR")

	if _, err := service.Update(context.Background(), uuid.New(), domain.UpdateVariantCommand{
		SKU: "  CREAM-50 ", Barcode: " 590 ", Price: mustMoney(1299, "EUR"), WeightGrams: 250,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	// An empty status defaults to active, matching creation.
	if repository.updated.SKU != "CREAM-50" || repository.updated.Barcode != "590" || repository.updated.Status != "active" {
		t.Fatalf("normalized command = %+v", repository.updated)
	}
}

func TestUpdateVariantRejectsNegativeWeight(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"en"}, "EUR")

	_, err := service.Update(context.Background(), uuid.New(), domain.UpdateVariantCommand{Price: mustMoney(100, "EUR"), WeightGrams: -1})
	if !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Update() error = %v, want ErrInvalidProduct", err)
	}
}

func TestArchiveVariantWithdrawsItFromSale(t *testing.T) {
	repository := &fakeVariantRepository{}
	service := NewVariantService(repository, []string{"en"}, "EUR")
	variantID := uuid.New()

	archived, err := service.Archive(context.Background(), variantID)
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if repository.archived != variantID || archived.Status != "archived" {
		t.Fatalf("Archive() = %+v, archived id = %s", archived, repository.archived)
	}
}

func TestVariantMutationsRequireAnIdentifier(t *testing.T) {
	service := NewVariantService(&fakeVariantRepository{}, []string{"en"}, "EUR")

	if _, err := service.Update(context.Background(), uuid.Nil, domain.UpdateVariantCommand{Price: mustMoney(100, "EUR")}); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Update(nil id) error = %v, want ErrInvalidProduct", err)
	}
	if _, err := service.Archive(context.Background(), uuid.Nil); !errors.Is(err, domain.ErrInvalidProduct) {
		t.Fatalf("Archive(nil id) error = %v, want ErrInvalidProduct", err)
	}
}

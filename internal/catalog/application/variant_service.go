package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

// VariantService owns validation of sellable catalog variants. The configured
// store currency is passed as a narrow value, not as global application config.
type VariantService struct {
	repo     domain.VariantRepository
	locales  localePolicy
	currency string
	fallback string
}

func NewVariantService(repo domain.VariantRepository, allowedLocales []string, currency string) *VariantService {
	return &VariantService{repo: repo, locales: newLocalePolicy(allowedLocales), currency: strings.ToUpper(strings.TrimSpace(currency))}
}

// WithFallbackLocale supplies the store's configured fallback so a variant
// without a translation in the requested locale can still be bought. Leaving
// it unset keeps the strict single-locale lookup.
func (s *VariantService) WithFallbackLocale(locale string) *VariantService {
	if normalized := normalize(locale); s.locales.allows(normalized) {
		s.fallback = normalized
	}
	return s
}

func (s *VariantService) Create(ctx context.Context, variant *domain.ProductVariant) error {
	if err := s.validate(variant); err != nil {
		return err
	}
	if variant.ID == uuid.Nil {
		variant.ID = uuid.New()
	}
	return s.repo.CreateVariant(ctx, variant)
}

func (s *VariantService) FindActiveForCheckout(ctx context.Context, variantID uuid.UUID, locale string) (*domain.CheckoutVariant, error) {
	if variantID == uuid.Nil || !s.locales.allows(normalize(locale)) {
		return nil, fmt.Errorf("%w: variant id and enabled locale are required", domain.ErrInvalidProduct)
	}
	// The fallback defaults to the requested locale, which keeps the lookup
	// strict for deployments that have not configured one.
	requested := normalize(locale)
	fallback := s.fallback
	if fallback == "" {
		fallback = requested
	}
	return s.repo.FindActiveForCheckout(ctx, variantID, requested, fallback)
}

func (s *VariantService) validate(variant *domain.ProductVariant) error {
	if variant == nil || variant.ProductID == uuid.Nil {
		return fmt.Errorf("%w: product variant and product id are required", domain.ErrInvalidProduct)
	}
	variant.SKU = strings.TrimSpace(variant.SKU)
	variant.Barcode = strings.TrimSpace(variant.Barcode)
	variant.Status = normalize(variant.Status)
	if variant.Status == "" {
		variant.Status = "active"
	}
	if variant.Status != "active" && variant.Status != "archived" {
		return fmt.Errorf("%w: unsupported variant status %q", domain.ErrInvalidProduct, variant.Status)
	}
	if err := variant.Price.Validate(); err != nil || variant.Price.Currency() != s.currency {
		return fmt.Errorf("%w: variant price must use configured currency %q", domain.ErrInvalidProduct, s.currency)
	}
	if variant.WeightGrams < 0 {
		return fmt.Errorf("%w: variant weight must not be negative", domain.ErrInvalidProduct)
	}
	if err := variant.ValidateOptionValues(); err != nil {
		return fmt.Errorf("%w: variant option values must belong to distinct options of this product", domain.ErrInvalidProduct)
	}
	return nil
}

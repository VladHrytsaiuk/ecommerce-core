package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

// VariantService owns validation of sellable catalog variants. The configured
// store currency is passed as a narrow value, not as global application config.
type VariantService struct {
	repo     domain.VariantRepository
	locales  localePolicy
	currency string
}

func NewVariantService(repo domain.VariantRepository, allowedLocales []string, currency string) *VariantService {
	return &VariantService{repo: repo, locales: newLocalePolicy(allowedLocales), currency: strings.ToUpper(strings.TrimSpace(currency))}
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
	return s.repo.FindActiveForCheckout(ctx, variantID, normalize(locale))
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
	if strings.TrimSpace(variant.Price.Currency) == "" {
		variant.Price.Currency = s.currency
	}
	price, err := money.New(variant.Price.Amount, variant.Price.Currency)
	if err != nil || price.Currency != s.currency {
		return fmt.Errorf("%w: variant price must use configured currency %q", domain.ErrInvalidProduct, s.currency)
	}
	if variant.WeightGrams < 0 {
		return fmt.Errorf("%w: variant weight must not be negative", domain.ErrInvalidProduct)
	}
	variant.Price = price
	return nil
}

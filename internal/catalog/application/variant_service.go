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

// Update replaces the mutable fields of an existing variant. It applies the
// same validation as creation, so a price cannot be edited into a currency the
// store does not sell in or a status the catalog does not recognise.
//
// Existing orders are unaffected: they carry an immutable price snapshot. A
// cart holding this variant re-reads the price when checkout prepares it, so
// the buyer pays the current one.
func (s *VariantService) Update(ctx context.Context, variantID uuid.UUID, command domain.UpdateVariantCommand) (*domain.ProductVariant, error) {
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("%w: variant id is required", domain.ErrInvalidProduct)
	}
	command.SKU = strings.TrimSpace(command.SKU)
	command.Barcode = strings.TrimSpace(command.Barcode)
	command.Status = normalize(command.Status)
	if command.Status == "" {
		command.Status = "active"
	}
	if command.Status != "active" && command.Status != "archived" {
		return nil, fmt.Errorf("%w: unsupported variant status %q", domain.ErrInvalidProduct, command.Status)
	}
	if err := command.Price.Validate(); err != nil || command.Price.Currency() != s.currency {
		return nil, fmt.Errorf("%w: variant price must use configured currency %q", domain.ErrInvalidProduct, s.currency)
	}
	if command.WeightGrams < 0 {
		return nil, fmt.Errorf("%w: variant weight must not be negative", domain.ErrInvalidProduct)
	}
	return s.repo.UpdateVariant(ctx, variantID, command)
}

// FindVariantForUpdate exposes the locked read the audited admin facade needs
// to record the state it replaced. It performs no validation of its own.
func (s *VariantService) FindVariantForUpdate(ctx context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("%w: variant id is required", domain.ErrInvalidProduct)
	}
	return s.repo.FindVariantForUpdate(ctx, variantID)
}

// Archive withdraws a variant from sale. It is the catalog's delete: see
// ArchiveVariant on the repository port for why a row removal is not offered.
func (s *VariantService) Archive(ctx context.Context, variantID uuid.UUID) (*domain.ProductVariant, error) {
	if variantID == uuid.Nil {
		return nil, fmt.Errorf("%w: variant id is required", domain.ErrInvalidProduct)
	}
	return s.repo.ArchiveVariant(ctx, variantID)
}

func (s *VariantService) FindActiveForCheckoutBatch(ctx context.Context, variantIDs []uuid.UUID, locale string) (map[uuid.UUID]domain.CheckoutVariant, error) {
	requested := normalize(locale)
	if len(variantIDs) == 0 || !s.locales.allows(requested) {
		return nil, fmt.Errorf("%w: variant ids and enabled locale are required", domain.ErrInvalidProduct)
	}
	unique := make([]uuid.UUID, 0, len(variantIDs))
	seen := make(map[uuid.UUID]struct{}, len(variantIDs))
	for _, variantID := range variantIDs {
		if variantID == uuid.Nil {
			return nil, fmt.Errorf("%w: variant id is required", domain.ErrInvalidProduct)
		}
		if _, exists := seen[variantID]; exists {
			continue
		}
		seen[variantID] = struct{}{}
		unique = append(unique, variantID)
	}
	// The fallback defaults to the requested locale, which keeps the lookup
	// strict for deployments that have not configured one.
	fallback := s.fallback
	if fallback == "" {
		fallback = requested
	}
	return s.repo.FindActiveForCheckoutBatch(ctx, unique, requested, fallback)
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

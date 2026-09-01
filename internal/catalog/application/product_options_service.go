package application

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

// ProductOptionsService validates the reusable option vocabulary before its
// repository makes the whole product-matrix mutation durable.
type ProductOptionsService struct {
	repo     domain.ProductOptionsRepository
	currency string
}

func NewProductOptionsService(repo domain.ProductOptionsRepository, currency string) *ProductOptionsService {
	return &ProductOptionsService{repo: repo, currency: strings.ToUpper(strings.TrimSpace(currency))}
}

func (s *ProductOptionsService) CreateProductOption(ctx context.Context, option *domain.ProductOption) error {
	if s == nil || s.repo == nil || option == nil || option.ProductID == uuid.Nil {
		return domain.ErrInvalidProduct
	}
	option.Name = strings.TrimSpace(option.Name)
	if option.Name == "" || utf8.RuneCountInString(option.Name) > 100 || option.Position < 0 || len(option.Values) == 0 {
		return domain.ErrInvalidProduct
	}
	seen := make(map[string]struct{}, len(option.Values))
	for index := range option.Values {
		value := &option.Values[index]
		value.Value = strings.TrimSpace(value.Value)
		if value.Value == "" || utf8.RuneCountInString(value.Value) > 255 || value.Position < 0 {
			return domain.ErrInvalidProduct
		}
		key := strings.ToLower(value.Value)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate option value", domain.ErrInvalidProduct)
		}
		seen[key] = struct{}{}
	}
	return s.repo.CreateProductOption(ctx, option)
}

func (s *ProductOptionsService) CreateVariant(ctx context.Context, variant *domain.ProductVariant, optionValueIDs []uuid.UUID) error {
	if s == nil || s.repo == nil || variant == nil || variant.ProductID == uuid.Nil || len(optionValueIDs) == 0 {
		return domain.ErrInvalidProduct
	}
	variant.SKU, variant.Barcode = strings.TrimSpace(variant.SKU), strings.TrimSpace(variant.Barcode)
	variant.Status = strings.ToLower(strings.TrimSpace(variant.Status))
	if variant.Status == "" {
		variant.Status = "active"
	}
	if (variant.Status != "active" && variant.Status != "archived") || variant.WeightGrams < 0 || variant.Price.Validate() != nil || variant.Price.Currency() != s.currency {
		return domain.ErrInvalidProduct
	}
	seen := make(map[uuid.UUID]struct{}, len(optionValueIDs))
	for _, id := range optionValueIDs {
		if id == uuid.Nil {
			return domain.ErrInvalidProduct
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: duplicate option value id", domain.ErrInvalidProduct)
		}
		seen[id] = struct{}{}
	}
	return s.repo.CreateVariantWithOptionValues(ctx, variant, optionValueIDs)
}

var _ domain.ProductOptionsService = (*ProductOptionsService)(nil)

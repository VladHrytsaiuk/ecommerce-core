package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

// ProductVariant is the sellable catalog unit. Its price always comes from
// the server-side catalog, never from a checkout request. SKU is an optional,
// externally assigned identifier; the core normalizes it but never invents a
// store-specific SKU format.
type ProductVariant struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	SKU       string
	Barcode   string
	Status    string
	Price     money.Money
	// OptionValues is the selected set for this SKU (for example Red + XL).
	// It is persisted by the Catalog options repository in the next phase;
	// ProductVariant remains the sole sellable/inventory unit.
	OptionValues []ProductOptionValue
	// IsAvailable is a read-model fact injected by a Catalog availability port;
	// it is never persisted on product_variants or trusted from an admin body.
	IsAvailable bool `gorm:"-"`
	// WeightGrams is zero only when the merchant has not supplied a shipping
	// weight yet. It is copied to an order snapshot at checkout.
	WeightGrams int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ValidateOptionValues protects command-side adapters before the database
// constraints run. A variant can select at most one value of each option and
// cannot repeat an option value in its own combination.
func (v ProductVariant) ValidateOptionValues() error {
	seenOptions := make(map[uuid.UUID]struct{}, len(v.OptionValues))
	seenValues := make(map[uuid.UUID]struct{}, len(v.OptionValues))
	for _, value := range v.OptionValues {
		if value.ID == uuid.Nil || value.OptionID == uuid.Nil || value.ProductID == uuid.Nil || value.ProductID != v.ProductID {
			return ErrInvalidProduct
		}
		if _, exists := seenOptions[value.OptionID]; exists {
			return ErrInvalidProduct
		}
		if _, exists := seenValues[value.ID]; exists {
			return ErrInvalidProduct
		}
		seenOptions[value.OptionID] = struct{}{}
		seenValues[value.ID] = struct{}{}
	}
	return nil
}

// CheckoutVariant is the immutable catalog snapshot data that Checkout needs
// to build an order item. It intentionally excludes mutable stock quantities.
type CheckoutVariant struct {
	VariantID   uuid.UUID
	ProductID   uuid.UUID
	SKU         string
	ProductName string
	UnitPrice   money.Money
	WeightGrams int
}

type VariantRepository interface {
	CreateVariant(context.Context, *ProductVariant) error
	FindActiveForCheckout(context.Context, uuid.UUID, string) (*CheckoutVariant, error)
}

type VariantService interface {
	Create(context.Context, *ProductVariant) error
	FindActiveForCheckout(context.Context, uuid.UUID, string) (*CheckoutVariant, error)
}

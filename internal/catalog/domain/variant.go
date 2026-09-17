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

// UpdateVariantCommand is the mutable surface of a sellable unit. Option
// values are deliberately absent: they define which variant this is, so
// changing them would silently turn "Red / XL" into a different SKU while
// carts, reservations and order snapshots still pointed at it.
type UpdateVariantCommand struct {
	SKU         string
	Barcode     string
	Status      string
	Price       money.Money
	WeightGrams int
}

type VariantRepository interface {
	CreateVariant(context.Context, *ProductVariant) error
	// FindVariantForUpdate reads a variant under a write lock so a caller can
	// record what it replaced without another writer moving it in between.
	FindVariantForUpdate(context.Context, uuid.UUID) (*ProductVariant, error)
	UpdateVariant(context.Context, uuid.UUID, UpdateVariantCommand) (*ProductVariant, error)
	// ArchiveVariant withdraws a variant from sale. There is deliberately no
	// hard delete: cart_items restricts it, wishlist and comparison rows would
	// be silently cascaded away, and stock, reservations, returns and
	// back-in-stock subscriptions reference variants without a foreign key, so
	// a row removal would orphan them without a single error. Archiving
	// withdraws the variant from every sale path while that history survives.
	ArchiveVariant(context.Context, uuid.UUID) (*ProductVariant, error)
	// FindActiveForCheckoutBatch resolves every requested variant in one
	// round trip. Checkout snapshots a whole cart at once, so a per-variant
	// lookup made database traffic scale with basket size on the most
	// latency-sensitive request in the store.
	//
	// Names resolve in the requested locale, falling back to the store's
	// configured locale when a variant has no translation yet: a missing
	// translation must never make an active, priced, in-stock product
	// unsellable. Variants that are absent or inactive are simply omitted.
	FindActiveForCheckoutBatch(ctx context.Context, variantIDs []uuid.UUID, locale, fallbackLocale string) (map[uuid.UUID]CheckoutVariant, error)
}

type VariantService interface {
	Create(context.Context, *ProductVariant) error
	Update(context.Context, uuid.UUID, UpdateVariantCommand) (*ProductVariant, error)
	Archive(context.Context, uuid.UUID) (*ProductVariant, error)
	FindActiveForCheckoutBatch(ctx context.Context, variantIDs []uuid.UUID, locale string) (map[uuid.UUID]CheckoutVariant, error)
}

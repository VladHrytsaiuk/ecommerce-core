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
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CheckoutVariant is the immutable catalog snapshot data that Checkout needs
// to build an order item. It intentionally excludes mutable stock quantities.
type CheckoutVariant struct {
	VariantID   uuid.UUID
	ProductID   uuid.UUID
	SKU         string
	ProductName string
	UnitPrice   money.Money
}

type VariantRepository interface {
	CreateVariant(context.Context, *ProductVariant) error
	FindActiveForCheckout(context.Context, uuid.UUID, string) (*CheckoutVariant, error)
}

type VariantService interface {
	Create(context.Context, *ProductVariant) error
	FindActiveForCheckout(context.Context, uuid.UUID, string) (*CheckoutVariant, error)
}

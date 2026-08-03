package domain

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/google/uuid"
)

const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusCancelled      = "cancelled"
)

type Item struct {
	VariantID   *uuid.UUID
	ProductName string
	SKU         string
	Quantity    int
	UnitPrice   money.Money
	Total       money.Money
}
type Draft struct {
	Number           string
	CustomerID       *uuid.UUID
	Subtotal         money.Money
	Tax              money.Money
	Total            money.Money
	PaymentProvider  string
	DeliveryProvider string
	Delivery         *DeliveryDetails
	Items            []Item
}

// DeliveryDetails is a provider-neutral immutable recipient snapshot. The
// provider code stays on the order; opaque location IDs are interpreted only
// by the selected Carrier adapter.
type DeliveryDetails struct {
	RecipientName  string
	RecipientPhone string
	CountryCode    string
	PostalCode     string
	City           string
	Line1          string
	Line2          string
	LocalityID     string
	ServicePointID string
}
type Order struct {
	ID               uuid.UUID
	Number           string
	CustomerID       *uuid.UUID
	Status           string
	Subtotal         money.Money
	Tax              money.Money
	Total            money.Money
	PaymentProvider  string
	DeliveryProvider string
	Delivery         *DeliveryDetails
	Items            []Item
	CreatedAt        time.Time
}
type Repository interface {
	Create(context.Context, *Order) error
}
type Service interface {
	Create(context.Context, Draft) (*Order, error)
}

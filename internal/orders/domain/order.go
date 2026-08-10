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
	VariantID       *uuid.UUID
	ProductName     string
	SKU             string
	Quantity        int
	UnitPrice       money.Money
	Total           money.Money
	UnitWeightGrams int
}
type Draft struct {
	Number           string
	CartID           uuid.UUID
	CustomerID       *uuid.UUID
	Subtotal         money.Money
	Tax              money.Money
	Shipping         money.Money
	Total            money.Money
	PaymentProvider  string
	DeliveryProvider string
	Delivery         *DeliveryDetails
	Items            []Item
	Promotion        *Promotion
	Contact          *ContactDetails
	ExpiresAt        time.Time
}

// Promotion is an immutable order snapshot of an applied promotion.
type Promotion struct {
	Code     string
	Type     string
	Value    int64
	Currency string
	Discount money.Money
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

// ContactDetails is the immutable receipt destination captured at checkout.
// It is separate from delivery details because digital orders may have no
// shipment and a customer can change their profile after payment.
type ContactDetails struct {
	Email  string
	Locale string
}

type Order struct {
	ID               uuid.UUID
	CartID           uuid.UUID
	Number           string
	CustomerID       *uuid.UUID
	Status           string
	Subtotal         money.Money
	Tax              money.Money
	Shipping         money.Money
	Total            money.Money
	PaymentProvider  string
	DeliveryProvider string
	Delivery         *DeliveryDetails
	Items            []Item
	Promotion        *Promotion
	Contact          *ContactDetails
	CreatedAt        time.Time
	ExpiresAt        time.Time
}
type Repository interface {
	Create(context.Context, *Order) error
}
type Service interface {
	Create(context.Context, Draft) (*Order, error)
}

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

type Line struct {
	VariantID   uuid.UUID
	WarehouseID uuid.UUID
	Quantity    int
}
type PrepareRequest struct {
	CheckoutID uuid.UUID
	Locale     string
	Lines      []Line
	ExpiresAt  time.Time
}
type PreparedCheckout struct {
	CheckoutID     uuid.UUID
	ReservationIDs []uuid.UUID
	ExpiresAt      time.Time
	Items          []ordersDomain.Item
	Subtotal       money.Money
	Tax            money.Money
	Shipping       money.Money
	Total          money.Money
}

type StartPaymentRequest struct {
	Preparation        PrepareRequest
	CartID             uuid.UUID
	OrderNumber        string
	CustomerID         *uuid.UUID
	CustomerPhone      string
	DeliveryProvider   string
	DeliveryOptionCode string
	Delivery           *DeliveryDetails
	ReturnURL          string
	CancelURL          string
}

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

type StartedCheckout struct {
	Prepared *PreparedCheckout
	Order    *ordersDomain.Order
	Session  paymentsDomain.PaymentSession
}

// DeliveryQuoteRequest is a read-only checkout operation. Lines are assembled
// by the HTTP adapter from the active Cart; callers must never trust browser
// supplied weight, price, or warehouse data.
type DeliveryQuoteRequest struct {
	Locale           string
	Lines            []Line
	DeliveryProvider string
	Delivery         DeliveryDetails
}

type DeliveryQuote struct {
	Provider string
	Options  []deliveryDomain.ShippingOption
}

type Service interface {
	PreparePayment(context.Context, PrepareRequest) (*PreparedCheckout, error)
	StartPayment(context.Context, StartPaymentRequest) (*StartedCheckout, error)
	QuoteDelivery(context.Context, DeliveryQuoteRequest) (*DeliveryQuote, error)
	ConfirmPayment(context.Context, workflowDomain.PaymentConfirmation) error
	CancelPayment(context.Context, uuid.UUID) error
}

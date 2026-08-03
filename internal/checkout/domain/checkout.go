package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
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
	Total          money.Money
}

type StartPaymentRequest struct {
	Preparation      PrepareRequest
	OrderNumber      string
	CustomerID       *uuid.UUID
	CustomerPhone    string
	DeliveryProvider string
	Delivery         *DeliveryDetails
	ReturnURL        string
	CancelURL        string
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

type Service interface {
	PreparePayment(context.Context, PrepareRequest) (*PreparedCheckout, error)
	StartPayment(context.Context, StartPaymentRequest) (*StartedCheckout, error)
	ConfirmPayment(context.Context, uuid.UUID) error
	CancelPayment(context.Context, uuid.UUID) error
}

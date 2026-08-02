package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
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
type Service interface {
	PreparePayment(context.Context, PrepareRequest) (*PreparedCheckout, error)
}

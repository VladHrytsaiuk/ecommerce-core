package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Line struct {
	VariantID   uuid.UUID
	WarehouseID uuid.UUID
	Quantity    int
}
type PrepareRequest struct {
	CheckoutID uuid.UUID
	Lines      []Line
	ExpiresAt  time.Time
}
type PreparedCheckout struct {
	CheckoutID     uuid.UUID
	ReservationIDs []uuid.UUID
	ExpiresAt      time.Time
}
type Service interface {
	PreparePayment(context.Context, PrepareRequest) (*PreparedCheckout, error)
}

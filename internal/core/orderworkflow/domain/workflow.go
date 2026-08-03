// Package domain defines the atomic order and reservation lifecycle.
package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

var (
	ErrReservationUnavailable = errors.New("order reservation is unavailable")
	ErrInvalidOrderTransition = errors.New("invalid order workflow transition")
	ErrPaymentMismatch        = errors.New("payment confirmation does not match pending order")
)

// PaymentAttempt is the durable, provider-neutral snapshot created after a
// gateway returns its reference. Client secrets and redirect form fields are
// deliberately excluded.
type PaymentAttempt struct {
	OrderID           uuid.UUID
	Provider          string
	ProviderReference string
	Amount            money.Money
}

// PaymentConfirmation comes from an already verified provider webhook. The
// PostgreSQL workflow validates it against PaymentAttempt in the same
// transaction that changes order and reservation state.
type PaymentConfirmation struct {
	PaymentAttempt
	Status string
}

type Repository interface {
	CreatePending(context.Context, *ordersDomain.Order, []uuid.UUID) error
	RegisterPayment(context.Context, PaymentAttempt) error
	CancelPending(context.Context, uuid.UUID) error
	MarkPaid(context.Context, PaymentConfirmation) error
	MarkFailed(context.Context, PaymentConfirmation) error
}

type Service interface {
	CreatePending(context.Context, ordersDomain.Draft, []uuid.UUID) (*ordersDomain.Order, error)
	RegisterPayment(context.Context, PaymentAttempt) error
	CancelPending(context.Context, uuid.UUID) error
	MarkPaid(context.Context, PaymentConfirmation) error
	MarkFailed(context.Context, PaymentConfirmation) error
}

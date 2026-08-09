// Package domain defines the atomic order and reservation lifecycle.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

var (
	ErrReservationUnavailable = errors.New("order reservation is unavailable")
	ErrInvalidOrderTransition = errors.New("invalid order workflow transition")
	ErrPaymentMismatch        = errors.New("payment confirmation does not match pending order")
)

// CheckoutAttemptRequest represents a request to durably record that an order
// is about to start the checkout process with a provider.
type CheckoutAttemptRequest struct {
	OrderID        uuid.UUID
	Provider       string
	IdempotencyKey string
	Amount         money.Money
	ExpiresAt      time.Time
}

// CheckoutAttempt represents an existing durable record of an attempt to
// start the checkout process.
type CheckoutAttempt struct {
	OrderID        uuid.UUID
	OrderNumber    string
	OrderStatus    string
	Provider       string
	IdempotencyKey string
	Amount         money.Money
	Status         string
	Attempts       int
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

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

// TransactionHook extends the atomic order workflow without allowing a module
// to own order or inventory persistence. The platform invokes it with a
// transaction-scoped context immediately before the surrounding transaction
// commits. Implementations must not perform external I/O.
type TransactionHook interface {
	BeforeCreatePending(context.Context, *ordersDomain.Order) error
	BeforeOrderTransition(context.Context, uuid.UUID, string) error
}

type Repository interface {
	CreatePending(context.Context, *ordersDomain.Order, []uuid.UUID) error
	CreatePendingCheckout(context.Context, *ordersDomain.Order, []uuid.UUID, CheckoutAttemptRequest) error
	CreatePaidCheckout(context.Context, *ordersDomain.Order, []uuid.UUID, CheckoutAttemptRequest) error
	RecordCheckoutAttempt(context.Context, CheckoutAttemptRequest) error
	FindCheckoutAttempt(context.Context, string) (*CheckoutAttempt, error)
	ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*CheckoutAttempt, error)
	ExpirePendingCheckout(context.Context, time.Time) (bool, error)
	RegisterPayment(context.Context, PaymentAttempt) error
	CancelPending(context.Context, uuid.UUID) error
	MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error
	RetryCheckoutAttempt(context.Context, uuid.UUID, error) error
	MarkPaid(context.Context, PaymentConfirmation) error
	MarkFailed(context.Context, PaymentConfirmation) error
}

type Service interface {
	CreatePending(context.Context, ordersDomain.Draft, []uuid.UUID) (*ordersDomain.Order, error)
	CreatePendingCheckout(context.Context, ordersDomain.Draft, []uuid.UUID, CheckoutAttemptRequest) (*ordersDomain.Order, error)
	CreatePaidCheckout(context.Context, ordersDomain.Draft, []uuid.UUID, CheckoutAttemptRequest) (*ordersDomain.Order, error)
	RecordCheckoutAttempt(context.Context, CheckoutAttemptRequest) error
	FindCheckoutAttempt(context.Context, string) (*CheckoutAttempt, error)
	ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*CheckoutAttempt, error)
	ExpirePendingCheckout(context.Context, time.Time) (bool, error)
	RegisterPayment(context.Context, PaymentAttempt) error
	CancelPending(context.Context, uuid.UUID) error
	MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error
	RetryCheckoutAttempt(context.Context, uuid.UUID, error) error
	MarkPaid(context.Context, PaymentConfirmation) error
	MarkFailed(context.Context, PaymentConfirmation) error
}

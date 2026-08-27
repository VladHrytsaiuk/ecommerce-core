// Package domain defines the atomic order and reservation lifecycle.
package domain

import (
	"context"
	"encoding/json"
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

// AdminCancellation records the trusted human actor responsible for a
// pending-payment cancellation. It is separate from generic cancellation so
// expiry/recovery workers remain explicitly system-originated in history.
type AdminCancellation struct {
	OrderID uuid.UUID
	ActorID uuid.UUID
	Reason  string
	EventID uuid.UUID
}

// OperationalStatusTransition is a non-financial order movement requested by
// a trusted back-office or carrier path. Payment, cancellation and refund
// transitions deliberately retain their dedicated workflow methods.
type OperationalStatusTransition struct {
	OrderID          uuid.UUID
	FromStatusCode   string
	ToStatusCode     string
	Trigger          ordersDomain.TransitionTrigger
	PaymentConfirmed bool
	TrackingNumber   string
	ActorType        ordersDomain.StatusActorType
	ActorID          *uuid.UUID
	Reason           string
	Metadata         json.RawMessage
	EventID          uuid.UUID
	OccurredAt       time.Time
}

// OperationalTransitionPolicy is implemented by the orders workflow module.
// Its methods operate using the same context-bound SQL transaction supplied by
// the cross-context workflow repository.
type OperationalTransitionPolicy interface {
	ValidateTransition(context.Context, ordersDomain.TransitionRequest) (string, error)
	AppendStatusHistory(context.Context, ordersDomain.OrderStatusHistory) (bool, error)
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
	MarkRefunded(context.Context, PaymentConfirmation) error
	CurrentStatusForUpdate(context.Context, uuid.UUID) (string, error)
	TransitionOperational(context.Context, OperationalStatusTransition) error
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
	MarkRefunded(context.Context, PaymentConfirmation) error
}

// OperationalService is intentionally separate from Service so Checkout and
// payment-webhook consumers do not gain an accidental dependency on the
// configurable back-office workflow surface.
type OperationalService interface {
	CurrentStatusForUpdate(context.Context, uuid.UUID) (string, error)
	TransitionOperational(context.Context, OperationalStatusTransition) error
}

// AdminCancellationRepository and AdminCancellationService are deliberately
// separate from the checkout/payment workflow ports. A caller must opt into
// carrying a trusted admin identity and reason; TTL and recovery callers never
// receive this capability by accident.
type AdminCancellationRepository interface {
	CancelPendingWithActor(context.Context, AdminCancellation) error
}

type AdminCancellationService interface {
	CancelPendingWithActor(context.Context, AdminCancellation) error
}

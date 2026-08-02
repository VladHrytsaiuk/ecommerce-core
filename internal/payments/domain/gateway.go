// Package domain defines provider-neutral payment contracts.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

// Gateway is implemented by an adapter, never by checkout or orders. Adapters
// translate an SDK/protocol request into these values and return neutral data.
type Gateway interface {
	Code() string
	CreateCheckout(context.Context, CheckoutPayment) (PaymentSession, error)
	VerifyWebhook(context.Context, WebhookRequest) (PaymentEvent, error)
	Refund(context.Context, RefundRequest) error
}

type CheckoutPayment struct {
	OrderID        uuid.UUID
	IdempotencyKey string
	Amount         money.Money
	ReturnURL      string
	CancelURL      string
}

type PaymentSession struct {
	ProviderReference string
	RedirectURL       string
	ExpiresAt         *time.Time
}

// WebhookRequest intentionally has no net/http dependency. The HTTP adapter
// owns HTTP request parsing and maps relevant headers into this value.
type WebhookRequest struct {
	RequestID string
	Headers   map[string]string
	Payload   []byte
}

type PaymentEvent struct {
	EventID           string
	ProviderReference string
	Status            string
	Amount            money.Money
	OccurredAt        time.Time
}

type RefundRequest struct {
	PaymentReference string
	Amount           money.Money
	IdempotencyKey   string
}

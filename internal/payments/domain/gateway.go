// Package domain defines provider-neutral payment contracts.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
)

var (
	ErrGatewayNotEnabled       = errors.New("payment gateway is not enabled")
	ErrUnsupportedEvent        = errors.New("unsupported payment event status")
	ErrInvalidWebhookSignature = errors.New("invalid payment webhook signature")
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
	// FormFields contains signed browser-post fields for provider-hosted
	// redirect checkouts. It is ephemeral just like ClientSecret.
	FormFields map[string]string
	// ClientSecret is returned by providers that require the storefront to
	// complete an in-page payment flow (for example, Stripe Payment Intents).
	// It is intentionally not persisted in the order and must never be logged.
	ClientSecret string
	ExpiresAt    *time.Time
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
	OrderID           uuid.UUID
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

// WebhookEventStore makes provider callbacks durable and idempotent. It does
// not contain provider payloads; adapters translate them to PaymentEvent first.
type WebhookEventStore interface {
	Claim(context.Context, string, PaymentEvent) (bool, error)
	MarkProcessed(context.Context, string, string) error
	Abandon(context.Context, string, string) error
}

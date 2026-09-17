package application

import (
	"context"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

func TestRegistrySelectsOnlyEnabledGateway(t *testing.T) {
	stripe := fakeGateway{code: "stripe"}
	liqPay := fakeGateway{code: "liqpay"}

	registry, err := NewRegistry([]string{"stripe"}, "stripe", stripe, liqPay)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if got := registry.Default(); got == nil || got.Code() != "stripe" {
		t.Fatalf("Default() = %v, want stripe", got)
	}
	if _, ok := registry.Get("liqpay"); ok {
		t.Fatal("disabled gateway was registered")
	}
}

func TestRegistryRejectsConfiguredGatewayWithoutAdapter(t *testing.T) {
	if _, err := NewRegistry([]string{"stripe"}, "stripe"); err == nil {
		t.Fatal("NewRegistry() error = nil, want missing adapter error")
	}
}

type fakeGateway struct{ code string }

func (g fakeGateway) Code() string { return g.code }
func (fakeGateway) CreateCheckout(context.Context, domain.CheckoutPayment) (domain.PaymentSession, error) {
	return domain.PaymentSession{}, nil
}
func (fakeGateway) VerifyWebhook(context.Context, domain.WebhookRequest) (domain.PaymentEvent, error) {
	return domain.PaymentEvent{}, nil
}
func (fakeGateway) Refund(context.Context, domain.RefundRequest) error { return nil }

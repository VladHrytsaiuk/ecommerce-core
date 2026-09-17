package app

import (
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

func TestNewPaymentRegistryBuildsOnlyConfiguredLiqPayAdapter(t *testing.T) {
	cfg := &config.Config{LiqPayPublicKey: "public", LiqPayPrivateKey: "private", LiqPayCallbackURL: "https://api.example.test/api/webhooks/payments/liqpay"}
	registry, err := newPaymentRegistry(cfg, StoreConfig{PaymentProviders: []string{"liqpay"}, PaymentDefault: "liqpay", PriceScale: 2})
	if err != nil {
		t.Fatalf("newPaymentRegistry() error = %v", err)
	}
	if gateway := registry.Default(); gateway == nil || gateway.Code() != "liqpay" {
		t.Fatalf("Default() = %v, want liqpay", gateway)
	}
}

func TestNewPaymentRegistryAllowsProviderFreeStore(t *testing.T) {
	registry, err := newPaymentRegistry(&config.Config{}, StoreConfig{})
	if err != nil {
		t.Fatalf("newPaymentRegistry() error = %v", err)
	}
	if registry.Default() != nil {
		t.Fatal("provider-free store unexpectedly has a default gateway")
	}
}

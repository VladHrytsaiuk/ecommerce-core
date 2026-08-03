package app

import (
	"strings"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

func TestNewStoreConfigAcceptsProviderFreeCore(t *testing.T) {
	cfg := validConfig()
	cfg.PaymentProviders, cfg.PaymentDefault = nil, ""
	cfg.ShippingProviders, cfg.ShippingDefault = nil, ""

	got, err := NewStoreConfig(cfg)

	if err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
	if got.PaymentDefault != "" || got.ShippingDefault != "" {
		t.Fatalf("unexpected provider defaults: %+v", got)
	}
}

func TestNewStoreConfigRejectsInvalidCombinations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Config)
		want   string
	}{
		{
			name:   "payment default is disabled",
			mutate: func(cfg *config.Config) { cfg.PaymentDefault = "stripe" },
			want:   "PAYMENT_DEFAULT",
		},
		{
			name: "enabled liqpay has no credentials",
			mutate: func(cfg *config.Config) {
				cfg.PaymentProviders, cfg.PaymentDefault = []string{"liqpay"}, "liqpay"
			},
			want: "LIQPAY_PUBLIC_KEY",
		},
		{
			name: "enabled stripe has no credentials",
			mutate: func(cfg *config.Config) {
				cfg.PaymentProviders, cfg.PaymentDefault = []string{"stripe"}, "stripe"
			},
			want: "STRIPE_SECRET_KEY",
		},
		{
			name: "enabled novaposhta has no credentials",
			mutate: func(cfg *config.Config) {
				cfg.ShippingProviders, cfg.ShippingDefault = []string{"novaposhta"}, "novaposhta"
			},
			want: "NOVA_POSHTA_API_KEY",
		},
		{
			name: "invalid locale",
			mutate: func(cfg *config.Config) {
				cfg.SupportedLocales = []string{"es$"}
				cfg.DefaultLocale = "es$"
				cfg.FallbackLocale = "es$"
			},
			want: "locale",
		},
		{
			name:   "duplicate locale",
			mutate: func(cfg *config.Config) { cfg.SupportedLocales = []string{"uk", "uk"} },
			want:   "duplicates",
		},
		{
			name:   "external inventory before sync exists",
			mutate: func(cfg *config.Config) { cfg.InventoryMode = "external_1c" },
			want:   "INVENTORY_MODE",
		},
		{name: "unsupported tax policy", mutate: func(cfg *config.Config) { cfg.TaxMode = "sales_tax" }, want: "TAX_MODE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)

			_, err := NewStoreConfig(cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewStoreConfig() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestNewStoreConfigAcceptsConfiguredLiqPay(t *testing.T) {
	cfg := validConfig()
	cfg.PaymentProviders, cfg.PaymentDefault = []string{"liqpay"}, "liqpay"
	cfg.LiqPayPublicKey, cfg.LiqPayPrivateKey = "public", "private"
	cfg.LiqPayCallbackURL = "https://api.example.test/api/webhooks/payments/liqpay"

	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
}

func TestNewStoreConfigAcceptsConfiguredStripe(t *testing.T) {
	cfg := validConfig()
	cfg.PaymentProviders, cfg.PaymentDefault = []string{"stripe"}, "stripe"
	cfg.StripeSecretKey, cfg.StripeWebhookSecret = "sk_test", "whsec_test"

	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
}

func TestNewPaymentRegistryBuildsOnlyEnabledStripeAdapter(t *testing.T) {
	cfg := validConfig()
	cfg.PaymentProviders, cfg.PaymentDefault = []string{"stripe"}, "stripe"
	cfg.StripeSecretKey, cfg.StripeWebhookSecret = "sk_test", "whsec_test"
	storeConfig, err := NewStoreConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := newPaymentRegistry(cfg, storeConfig)
	if err != nil {
		t.Fatalf("newPaymentRegistry() error = %v", err)
	}
	if registry.Default() == nil || registry.Default().Code() != "stripe" {
		t.Fatalf("default gateway = %v", registry.Default())
	}
}

func TestNewStoreConfigAcceptsConfiguredNovaPoshta(t *testing.T) {
	cfg := validConfig()
	cfg.ShippingProviders, cfg.ShippingDefault = []string{"novaposhta"}, "novaposhta"
	cfg.NovaPoshtaAPIKey = "api-key"
	cfg.NPSenderRef = "sender"
	cfg.NPSenderCityRef = "sender-city"
	cfg.NPSenderAddressRef = "sender-address"
	cfg.NPContactSenderRef = "sender-contact"

	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
}

func TestNewDeliveryRegistryBuildsOnlyEnabledNovaPoshtaAdapter(t *testing.T) {
	cfg := validConfig()
	cfg.ShippingProviders, cfg.ShippingDefault = []string{"novaposhta"}, "novaposhta"
	cfg.NovaPoshtaAPIKey = "api-key"
	cfg.NovaPoshtaURL = "https://api.example.test/novaposhta"
	cfg.NPSenderRef = "sender"
	cfg.NPSenderCityRef = "sender-city"
	cfg.NPSenderAddressRef = "sender-address"
	cfg.NPContactSenderRef = "sender-contact"
	storeConfig, err := NewStoreConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := newDeliveryRegistry(cfg, storeConfig)
	if err != nil {
		t.Fatalf("newDeliveryRegistry() error = %v", err)
	}
	if registry.Default() == nil || registry.Default().Code() != "novaposhta" {
		t.Fatalf("default carrier = %v", registry.Default())
	}
}

func TestNewStoreConfigAcceptsVATAndNonUAHCurrency(t *testing.T) {
	cfg := validConfig()
	cfg.Currency, cfg.PriceScale = "EUR", 2
	cfg.TaxMode, cfg.VATRate = "vat_included", 21

	if _, err := NewStoreConfig(cfg); err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
}

func TestNewStoreConfigAcceptsThreeConfiguredLocales(t *testing.T) {
	cfg := validConfig()
	cfg.SupportedLocales = []string{"es", "en", "ca"}
	cfg.DefaultLocale = "es"
	cfg.FallbackLocale = "es"

	got, err := NewStoreConfig(cfg)
	if err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
	if len(got.SupportedLocales) != 3 {
		t.Fatalf("SupportedLocales = %v, want three locales", got.SupportedLocales)
	}
}

func TestBootstrapFailsBeforeBuildingDependenciesForInvalidConfig(t *testing.T) {
	cfg := validConfig()
	cfg.PaymentDefault = "stripe"

	application, err := Bootstrap(cfg, StoreConfig{}, nil, nil)
	if err == nil || application != nil {
		t.Fatalf("Bootstrap() = (%v, %v), want (nil, validation error)", application, err)
	}
}

func validConfig() *config.Config {
	return &config.Config{
		StoreCode: "default-store", StoreName: "ecommerce-core store",
		DefaultLocale: "uk", SupportedLocales: []string{"uk", "en"}, FallbackLocale: "uk",
		Currency: "UAH", PriceScale: 2, TaxMode: "none", VATRate: 0,
		PaymentProviders: nil, PaymentDefault: "",
		ShippingProviders: nil, ShippingDefault: "",
		InventoryMode:          "internal",
		CheckoutReservationTTL: 15 * time.Minute,
	}
}

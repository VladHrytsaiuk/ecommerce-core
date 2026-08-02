package app

import (
	"strings"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

func TestNewStoreConfigAcceptsCurrentCompatibilityDefaults(t *testing.T) {
	cfg := validConfig()

	got, err := NewStoreConfig(cfg)

	if err != nil {
		t.Fatalf("NewStoreConfig() error = %v", err)
	}
	if got.PaymentDefault != "liqpay" || got.ShippingDefault != "novaposhta" {
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
			name: "missing payment credentials",
			mutate: func(cfg *config.Config) {
				cfg.LiqPayPublicKey = ""
				cfg.LiqPayPrivateKey = ""
			},
			want: "LIQPAY_PUBLIC_KEY",
		},
		{
			name:   "missing delivery credentials",
			mutate: func(cfg *config.Config) { cfg.NovaPoshtaAPIKey = "" },
			want:   "NOVA_POSHTA_API_KEY",
		},
		{
			name:   "external inventory before sync exists",
			mutate: func(cfg *config.Config) { cfg.InventoryMode = "external_1c" },
			want:   "INVENTORY_MODE",
		},
		{
			name: "tax policy is not implemented",
			mutate: func(cfg *config.Config) {
				cfg.TaxMode = "vat_included"
				cfg.VATRate = 21
			},
			want: "TAX_MODE",
		},
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
		PaymentProviders: []string{"liqpay"}, PaymentDefault: "liqpay",
		ShippingProviders: []string{"novaposhta"}, ShippingDefault: "novaposhta",
		InventoryMode:   "internal",
		LiqPayPublicKey: "public", LiqPayPrivateKey: "private", NovaPoshtaAPIKey: "key",
	}
}

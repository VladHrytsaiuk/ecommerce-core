package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

var storeCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// StoreConfig is the normalized, provider-neutral configuration consumed by the
// Composition Root. It coexists with platform/config.Config while legacy
// services are migrated to narrow policies.
type StoreConfig struct {
	Code                 string
	Name                 string
	DefaultLocale        string
	SupportedLocales     []string
	FallbackLocale       string
	Currency             string
	PriceScale           int
	TaxMode              string
	VATRate              int
	PaymentProviders     []string
	PaymentDefault       string
	ShippingProviders    []string
	ShippingDefault      string
	InventoryMode        string
	EnabledModules       []string
	CheckoutAllowGuest   bool
	CheckoutRequirePhone bool
}

// NewStoreConfig maps environment-loaded configuration into the typed
// application configuration and validates only capabilities implemented today.
func NewStoreConfig(cfg *config.Config) (StoreConfig, error) {
	storeConfig := StoreConfig{
		Code: cfg.StoreCode, Name: cfg.StoreName,
		DefaultLocale:    normalize(cfg.DefaultLocale),
		SupportedLocales: normalizeAll(cfg.SupportedLocales),
		FallbackLocale:   normalize(cfg.FallbackLocale),
		Currency:         strings.ToUpper(strings.TrimSpace(cfg.Currency)),
		PriceScale:       cfg.PriceScale, TaxMode: normalize(cfg.TaxMode), VATRate: cfg.VATRate,
		PaymentProviders:     normalizeAll(cfg.PaymentProviders),
		PaymentDefault:       normalize(cfg.PaymentDefault),
		ShippingProviders:    normalizeAll(cfg.ShippingProviders),
		ShippingDefault:      normalize(cfg.ShippingDefault),
		InventoryMode:        normalize(cfg.InventoryMode),
		EnabledModules:       normalizeAll(cfg.EnabledModules),
		CheckoutAllowGuest:   cfg.CheckoutAllowGuest,
		CheckoutRequirePhone: cfg.CheckoutRequirePhone,
	}
	return storeConfig, storeConfig.Validate()
}

// Validate fails before the HTTP server starts. The allow-lists deliberately
// reflect adapters implemented in the current application graph; future phases
// extend the factories and these lists together.
func (c StoreConfig) Validate() error {
	if !storeCodePattern.MatchString(c.Code) {
		return fmt.Errorf("STORE_CODE must contain lowercase letters, digits, or hyphens")
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("STORE_NAME must not be empty")
	}
	if len(c.SupportedLocales) == 0 {
		return fmt.Errorf("SUPPORTED_LOCALES must contain at least one locale")
	}
	if !contains(c.SupportedLocales, c.DefaultLocale) {
		return fmt.Errorf("DEFAULT_LOCALE %q is not in SUPPORTED_LOCALES", c.DefaultLocale)
	}
	if !contains(c.SupportedLocales, c.FallbackLocale) {
		return fmt.Errorf("FALLBACK_LOCALE %q is not in SUPPORTED_LOCALES", c.FallbackLocale)
	}
	for _, locale := range c.SupportedLocales {
		if !isSupportedLocale(locale) {
			return fmt.Errorf("locale %q is not supported by the current catalog and HTTP compatibility layer", locale)
		}
	}
	if c.Currency != "UAH" {
		return fmt.Errorf("CURRENCY %q is not supported by the current money implementation", c.Currency)
	}
	if c.PriceScale != 2 {
		return fmt.Errorf("PRICE_SCALE %d is not supported by the current money implementation", c.PriceScale)
	}
	if c.TaxMode != "none" {
		return fmt.Errorf("TAX_MODE %q is not implemented yet", c.TaxMode)
	}
	if c.VATRate != 0 {
		return fmt.Errorf("VAT_RATE requires an implemented tax policy")
	}
	if !contains(c.PaymentProviders, c.PaymentDefault) {
		return fmt.Errorf("PAYMENT_DEFAULT %q is not enabled", c.PaymentDefault)
	}
	if !contains(c.ShippingProviders, c.ShippingDefault) {
		return fmt.Errorf("SHIPPING_DEFAULT %q is not enabled", c.ShippingDefault)
	}
	if !allEqual(c.PaymentProviders, "liqpay") {
		return fmt.Errorf("only the liqpay payment adapter is implemented")
	}
	if !allEqual(c.ShippingProviders, "novaposhta") {
		return fmt.Errorf("only the novaposhta delivery adapter is implemented")
	}
	if c.InventoryMode != "internal" {
		return fmt.Errorf("INVENTORY_MODE %q is not implemented yet", c.InventoryMode)
	}
	return nil
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeAll(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalize(value); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func allEqual(values []string, expected string) bool {
	return len(values) > 0 && func() bool {
		for _, value := range values {
			if value != expected {
				return false
			}
		}
		return true
	}()
}

func isSupportedLocale(locale string) bool {
	return locale == "uk" || locale == "en"
}

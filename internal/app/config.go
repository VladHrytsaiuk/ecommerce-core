package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

var storeCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var localeCodePattern = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)

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
		Code: normalize(cfg.StoreCode), Name: strings.TrimSpace(cfg.StoreName),
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
	if hasDuplicates(c.SupportedLocales) {
		return fmt.Errorf("SUPPORTED_LOCALES must not contain duplicates")
	}
	if !contains(c.SupportedLocales, c.DefaultLocale) {
		return fmt.Errorf("DEFAULT_LOCALE %q is not in SUPPORTED_LOCALES", c.DefaultLocale)
	}
	if !contains(c.SupportedLocales, c.FallbackLocale) {
		return fmt.Errorf("FALLBACK_LOCALE %q is not in SUPPORTED_LOCALES", c.FallbackLocale)
	}
	for _, locale := range c.SupportedLocales {
		if !isValidLocale(locale) {
			return fmt.Errorf("locale %q must be a valid lowercase locale code up to 10 characters", locale)
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
	if len(c.PaymentProviders) > 0 && !contains(c.PaymentProviders, c.PaymentDefault) {
		return fmt.Errorf("PAYMENT_DEFAULT %q is not enabled", c.PaymentDefault)
	}
	if len(c.PaymentProviders) == 0 && c.PaymentDefault != "" {
		return fmt.Errorf("PAYMENT_DEFAULT requires an enabled payment provider")
	}
	if len(c.ShippingProviders) > 0 && !contains(c.ShippingProviders, c.ShippingDefault) {
		return fmt.Errorf("SHIPPING_DEFAULT %q is not enabled", c.ShippingDefault)
	}
	if len(c.ShippingProviders) == 0 && c.ShippingDefault != "" {
		return fmt.Errorf("SHIPPING_DEFAULT requires an enabled shipping provider")
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

func hasDuplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func isValidLocale(locale string) bool {
	return len(locale) <= 10 && localeCodePattern.MatchString(locale)
}

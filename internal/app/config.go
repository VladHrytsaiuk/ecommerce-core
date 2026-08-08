package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

var storeCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var localeCodePattern = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)
var profileFieldKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// StoreConfig is the normalized, provider-neutral configuration consumed by the
// Composition Root. It coexists with platform/config.Config while legacy
// services are migrated to narrow policies.
type StoreConfig struct {
	Code                   string
	Name                   string
	DefaultLocale          string
	SupportedLocales       []string
	FallbackLocale         string
	Currency               string
	PriceScale             int
	TaxMode                string
	VATRate                int
	PaymentProviders       []string
	PaymentDefault         string
	ShippingProviders      []string
	ShippingDefault        string
	InventoryMode          string
	EnabledModules         []string
	CheckoutAllowGuest     bool
	CheckoutRequirePhone   bool
	CheckoutReservationTTL time.Duration
	DefaultWarehouseID     uuid.UUID
	GoogleOAuth            *GoogleOAuthConfig
	OAuthAttemptTTL        time.Duration
	ProfilePolicy          *identityDomain.ProfilePolicy
	ComparisonMaxItems     int
}

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// NewStoreConfig maps environment-loaded configuration into the typed
// application configuration and validates only capabilities implemented today.
func NewStoreConfig(cfg *config.Config) (StoreConfig, error) {
	defaultWarehouseID, err := uuid.Parse(strings.TrimSpace(cfg.DefaultWarehouseID))
	if err != nil || defaultWarehouseID == uuid.Nil {
		return StoreConfig{}, fmt.Errorf("DEFAULT_WAREHOUSE_ID must be a non-empty UUID")
	}
	profilePolicy, err := parseProfilePolicy(cfg.ProfilePolicyJSON)
	if err != nil {
		return StoreConfig{}, err
	}
	storeConfig := StoreConfig{
		Code: normalize(cfg.StoreCode), Name: strings.TrimSpace(cfg.StoreName),
		DefaultLocale:    normalize(cfg.DefaultLocale),
		SupportedLocales: normalizeAll(cfg.SupportedLocales),
		FallbackLocale:   normalize(cfg.FallbackLocale),
		Currency:         strings.ToUpper(strings.TrimSpace(cfg.Currency)),
		PriceScale:       cfg.PriceScale, TaxMode: normalize(cfg.TaxMode), VATRate: cfg.VATRate,
		PaymentProviders:       normalizeAll(cfg.PaymentProviders),
		PaymentDefault:         normalize(cfg.PaymentDefault),
		ShippingProviders:      normalizeAll(cfg.ShippingProviders),
		ShippingDefault:        normalize(cfg.ShippingDefault),
		InventoryMode:          normalize(cfg.InventoryMode),
		EnabledModules:         normalizeAll(cfg.EnabledModules),
		CheckoutAllowGuest:     cfg.CheckoutAllowGuest,
		CheckoutRequirePhone:   cfg.CheckoutRequirePhone,
		CheckoutReservationTTL: cfg.CheckoutReservationTTL,
		DefaultWarehouseID:     defaultWarehouseID,
		OAuthAttemptTTL:        cfg.OAuthAttemptTTL,
		ProfilePolicy:          profilePolicy,
		ComparisonMaxItems:     cfg.ComparisonMaxItems,
	}
	if strings.TrimSpace(cfg.GoogleClientID) != "" || strings.TrimSpace(cfg.GoogleClientSecret) != "" || strings.TrimSpace(cfg.GoogleRedirectURI) != "" {
		storeConfig.GoogleOAuth = &GoogleOAuthConfig{ClientID: strings.TrimSpace(cfg.GoogleClientID), ClientSecret: strings.TrimSpace(cfg.GoogleClientSecret), RedirectURI: strings.TrimSpace(cfg.GoogleRedirectURI)}
	}
	if err := storeConfig.Validate(); err != nil {
		return StoreConfig{}, err
	}
	if err := validateEnabledAdapters(cfg, storeConfig); err != nil {
		return StoreConfig{}, err
	}
	return storeConfig, nil
}

// validateEnabledAdapters is intentionally kept at the typed-config boundary:
// a deployment with an enabled provider but missing credentials fails before
// database connection or HTTP startup. Adapter construction remains in
// Bootstrap.
func validateEnabledAdapters(cfg *config.Config, storeConfig StoreConfig) error {
	if storeConfig.GoogleOAuth != nil {
		if storeConfig.GoogleOAuth.ClientID == "" || storeConfig.GoogleOAuth.ClientSecret == "" || storeConfig.GoogleOAuth.RedirectURI == "" {
			return fmt.Errorf("GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET and GOOGLE_REDIRECT_URI must all be set when Google OAuth is configured")
		}
	}
	for _, provider := range storeConfig.PaymentProviders {
		switch provider {
		case "liqpay":
			if strings.TrimSpace(cfg.LiqPayPublicKey) == "" || strings.TrimSpace(cfg.LiqPayPrivateKey) == "" {
				return fmt.Errorf("LIQPAY_PUBLIC_KEY and LIQPAY_PRIVATE_KEY are required when liqpay is enabled")
			}
			if strings.TrimSpace(cfg.LiqPayCallbackURL) == "" {
				return fmt.Errorf("LIQPAY_CALLBACK_URL is required when liqpay is enabled")
			}
		case "stripe":
			if strings.TrimSpace(cfg.StripeSecretKey) == "" || strings.TrimSpace(cfg.StripeWebhookSecret) == "" {
				return fmt.Errorf("STRIPE_SECRET_KEY and STRIPE_WEBHOOK_SECRET are required when stripe is enabled")
			}
		case "redsys":
			if strings.TrimSpace(cfg.RedsysMerchantCode) == "" || strings.TrimSpace(cfg.RedsysTerminal) == "" || strings.TrimSpace(cfg.RedsysSecretKey) == "" || strings.TrimSpace(cfg.RedsysCallbackURL) == "" || strings.TrimSpace(cfg.RedsysCurrencyCode) == "" {
				return fmt.Errorf("REDSYS_MERCHANT_CODE, REDSYS_TERMINAL, REDSYS_SECRET_KEY, REDSYS_CALLBACK_URL and REDSYS_CURRENCY_CODE are required when redsys is enabled")
			}
		default:
			return fmt.Errorf("PAYMENT_PROVIDERS contains unsupported provider %q", provider)
		}
	}
	for _, provider := range storeConfig.ShippingProviders {
		switch provider {
		case "novaposhta":
			if strings.TrimSpace(cfg.NovaPoshtaAPIKey) == "" || strings.TrimSpace(cfg.NPSenderRef) == "" || strings.TrimSpace(cfg.NPSenderCityRef) == "" || strings.TrimSpace(cfg.NPSenderAddressRef) == "" || strings.TrimSpace(cfg.NPContactSenderRef) == "" {
				return fmt.Errorf("NOVA_POSHTA_API_KEY and NP sender references are required when novaposhta is enabled")
			}
		case "dhlexpress":
			if strings.TrimSpace(cfg.DHLExpressUsername) == "" || strings.TrimSpace(cfg.DHLExpressPassword) == "" || strings.TrimSpace(cfg.DHLExpressAccountNumber) == "" || strings.TrimSpace(cfg.DHLExpressProductCode) == "" {
				return fmt.Errorf("DHL_EXPRESS_USERNAME, DHL_EXPRESS_PASSWORD, DHL_EXPRESS_ACCOUNT_NUMBER and DHL_EXPRESS_PRODUCT_CODE are required when dhlexpress is enabled")
			}
			if strings.TrimSpace(cfg.DHLExpressSenderName) == "" || strings.TrimSpace(cfg.DHLExpressSenderPhone) == "" || strings.TrimSpace(cfg.DHLExpressSenderCountry) == "" || strings.TrimSpace(cfg.DHLExpressSenderPostal) == "" || strings.TrimSpace(cfg.DHLExpressSenderCity) == "" || strings.TrimSpace(cfg.DHLExpressSenderLine1) == "" {
				return fmt.Errorf("complete DHL_EXPRESS_SENDER_* address is required when dhlexpress is enabled")
			}
			if cfg.DHLExpressPackageLength <= 0 || cfg.DHLExpressPackageWidth <= 0 || cfg.DHLExpressPackageHeight <= 0 {
				return fmt.Errorf("DHL_EXPRESS_PACKAGE_LENGTH_CM, DHL_EXPRESS_PACKAGE_WIDTH_CM and DHL_EXPRESS_PACKAGE_HEIGHT_CM must be positive when dhlexpress is enabled")
			}
		default:
			return fmt.Errorf("SHIPPING_PROVIDERS contains unsupported provider %q", provider)
		}
	}
	return nil
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
	if _, err := money.New(0, c.Currency); err != nil {
		return fmt.Errorf("CURRENCY %q must be a three-letter ISO 4217 code", c.Currency)
	}
	if c.PriceScale < 0 || c.PriceScale > 6 {
		return fmt.Errorf("PRICE_SCALE %d must be between 0 and 6", c.PriceScale)
	}
	if _, err := tax.NewPolicy(tax.Mode(c.TaxMode), c.VATRate); err != nil {
		return err
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
	if contains(c.ShippingProviders, "novaposhta") && c.Currency != "UAH" {
		return fmt.Errorf("novaposhta requires CURRENCY=UAH")
	}
	if len(c.ShippingProviders) == 0 && c.ShippingDefault != "" {
		return fmt.Errorf("SHIPPING_DEFAULT requires an enabled shipping provider")
	}
	if c.InventoryMode != "internal" {
		return fmt.Errorf("INVENTORY_MODE %q is not implemented yet", c.InventoryMode)
	}
	if !contains(c.EnabledModules, "inventory") {
		return fmt.Errorf("ENABLED_MODULES must include inventory because checkout reservations require it")
	}
	if c.CheckoutReservationTTL <= 0 || c.CheckoutReservationTTL > 24*time.Hour {
		return fmt.Errorf("CHECKOUT_RESERVATION_TTL must be between 1ns and 24h")
	}
	if c.DefaultWarehouseID == uuid.Nil {
		return fmt.Errorf("DEFAULT_WAREHOUSE_ID must be a non-empty UUID")
	}
	if c.OAuthAttemptTTL <= 0 || c.OAuthAttemptTTL > time.Hour {
		return fmt.Errorf("OAUTH_ATTEMPT_TTL must be between 1ns and 1h")
	}
	if contains(c.EnabledModules, "user_profiles") && c.ProfilePolicy == nil {
		return fmt.Errorf("PROFILE_POLICY_JSON is required when user_profiles is enabled")
	}
	if !contains(c.EnabledModules, "user_profiles") && c.ProfilePolicy != nil {
		return fmt.Errorf("PROFILE_POLICY_JSON requires ENABLED_MODULES to include user_profiles")
	}
	if contains(c.EnabledModules, "comparison") && (c.ComparisonMaxItems < 1 || c.ComparisonMaxItems > 100) {
		return fmt.Errorf("COMPARISON_MAX_ITEMS must be between 1 and 100 when comparison is enabled")
	}
	return nil
}

func parseProfilePolicy(raw string) (*identityDomain.ProfilePolicy, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var policy identityDomain.ProfilePolicy
	if err := decoder.Decode(&policy); err != nil {
		return nil, fmt.Errorf("PROFILE_POLICY_JSON is invalid: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || policy.SchemaVersion <= 0 {
		return nil, fmt.Errorf("PROFILE_POLICY_JSON must contain one object with a positive schema_version")
	}
	seen := make(map[string]struct{}, len(policy.Fields))
	for _, field := range policy.Fields {
		key := strings.TrimSpace(field.Key)
		if !profileFieldKeyPattern.MatchString(key) || key != field.Key {
			return nil, fmt.Errorf("PROFILE_POLICY_JSON field key %q is invalid", field.Key)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("PROFILE_POLICY_JSON field key %q is duplicated", key)
		}
		seen[key] = struct{}{}
		switch field.Type {
		case identityDomain.ProfileFieldString, identityDomain.ProfileFieldNumber, identityDomain.ProfileFieldDate, identityDomain.ProfileFieldBool, identityDomain.ProfileFieldEnum:
		default:
			return nil, fmt.Errorf("PROFILE_POLICY_JSON field %q has unsupported type %q", key, field.Type)
		}
		if field.MaxLength < 0 || field.MaxLength > 4096 {
			return nil, fmt.Errorf("PROFILE_POLICY_JSON field %q has invalid max_length", key)
		}
		if field.Type == identityDomain.ProfileFieldEnum && len(field.AllowedValues) == 0 {
			return nil, fmt.Errorf("PROFILE_POLICY_JSON enum field %q requires allowed_values", key)
		}
	}
	return &policy, nil
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

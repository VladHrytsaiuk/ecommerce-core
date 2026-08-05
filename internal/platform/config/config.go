// Package config відповідає за завантаження та зберігання
// всіх налаштувань середовища (environment variables) для нашого застосунку.
package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const insecureDefaultJWTSecret = "very_secret_key_change_me_in_prod"

// Config містить основні налаштування для запуску сервера та підключення до БД.
type Config struct {
	Port                 string
	DBURL                string
	JWTSecret            string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	FrontendURL          string
	GoogleClientID       string
	MaxSessions          int
	Env                  string
	CookieSecure         bool
	APIRateLimitPerMin   int
	SensitiveRatePerMin  int

	// HTTP server
	// RequestTimeout — верхня межа тривалості обробки HTTP-запиту (context deadline).
	// Має бути достатньою для повільних операцій (напр., завантаження кількох зображень у Cloudinary).
	RequestTimeout time.Duration

	// SMTP (Email)
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// SendGrid (Email)
	SendGridAPIKey string
	EmailFrom      string

	// Nova Poshta (Shipping)
	NovaPoshtaAPIKey string
	NovaPoshtaURL    string

	// Media Storage
	CloudinaryURL string
	StoreLogoURL  string

	// API Host (for Swagger)
	APIHost string

	// OTP Send Rate Limiting
	OTPSendRateLimit    int
	OTPSendRateInterval time.Duration

	// Email Send Rate Limiting (Feedback)
	EmailRateLimit    int
	EmailRateInterval time.Duration

	// Trusted Proxies
	TrustedProxies []string
	// Dynamic Badges
	BadgeNewDays             int
	BadgeBestSellerThreshold int

	// Order / Payment
	AdminNotificationEmail string
	LiqPayPublicKey        string
	LiqPayPrivateKey       string
	LiqPayCallbackURL      string
	StripeSecretKey        string
	StripeWebhookSecret    string
	RedsysMerchantCode     string
	RedsysTerminal         string
	RedsysSecretKey        string
	RedsysCallbackURL      string
	RedsysCurrencyCode     string

	// Nova Poshta Sender (for TTN creation)
	NPSenderRef               string
	NPSenderCityRef           string
	NPSenderAddressRef        string
	NPContactSenderRef        string
	NPSenderPhone             string
	NPTrackingIntervalMinutes int

	// Manager
	ManagerBaseURL string

	// Vodafone OBM (SMS / верифікація телефону)
	OBMBaseURL         string
	OBMTokenPath       string
	OBMBasicAuthHeader string
	OBMUsername        string
	OBMPassword        string
	OBMSenderID        int
	OBMDistributionID  string
	OBMValidityMinutes string
	OBMStatusCheck     bool
	PhoneCodeTTL       time.Duration
	CORSAllowOrigins   []string

	// Store configuration. These fields are consumed by internal/app during the
	// Strangler Fig migration; legacy services continue to use the fields above.
	StoreCode              string
	StoreName              string
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
	DefaultWarehouseID     string
}

// Load читає файл .env (якщо він існує) та повертає готову структуру Config.
// Якщо обов'язкові змінні (наприклад, DB_URL) відсутні, програма завершить роботу з помилкою (log.Fatal).
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Info: .env file not found, using system environment variables")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("Fatal: DB_URL environment variable is not set")
	}

	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))

	accessTokenDurationStr := os.Getenv("ACCESS_TOKEN_DURATION")
	if accessTokenDurationStr == "" {
		accessTokenDurationStr = "15m"
	}
	accessTokenDuration, err := time.ParseDuration(accessTokenDurationStr)
	if err != nil {
		log.Fatalf("Fatal: Invalid ACCESS_TOKEN_DURATION format: %v", err)
	}

	refreshTokenDurationStr := os.Getenv("REFRESH_TOKEN_DURATION")
	if refreshTokenDurationStr == "" {
		refreshTokenDurationStr = "168h"
	}
	refreshTokenDuration, err := time.ParseDuration(refreshTokenDurationStr)
	if err != nil {
		log.Fatalf("Fatal: Invalid REFRESH_TOKEN_DURATION format: %v", err)
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	if googleClientID == "" {
		log.Println("Warning: GOOGLE_CLIENT_ID is not set. Google Auth will not work.")
	}

	maxSessionsStr := os.Getenv("MAX_SESSIONS")
	if maxSessionsStr == "" {
		maxSessionsStr = "10"
	}
	maxSessions, err := strconv.Atoi(maxSessionsStr)
	if err != nil {
		log.Printf("Warning: Invalid MAX_SESSIONS: %v. Using default: 10", err)
		maxSessions = 10
	}

	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "" {
		appEnv = "production"
	}

	// Таймаут обробки HTTP-запиту. За замовчуванням 30s — достатньо для завантаження
	// кількох зображень у Cloudinary в межах одного запиту (створення/оновлення товару).
	requestTimeout := 30 * time.Second
	if v := os.Getenv("HTTP_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			requestTimeout = d
		} else {
			log.Printf("Warning: Invalid HTTP_REQUEST_TIMEOUT: %v. Using default: 30s", err)
		}
	}

	// SMTP Config
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPortStr := os.Getenv("SMTP_PORT")
	smtpPort := 587
	if smtpPortStr != "" {
		smtpPort, err = strconv.Atoi(smtpPortStr)
		if err != nil {
			log.Printf("Warning: Invalid SMTP_PORT: %v. Using default: 587", err)
			smtpPort = 587
		}
	}
	smtpUser := os.Getenv("SMTP_USER")
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		smtpFrom = smtpUser
	}

	// SendGrid Config
	sendGridAPIKey := os.Getenv("SENDGRID_API_KEY")
	emailFrom := os.Getenv("EMAIL_FROM")

	// Nova Poshta Config
	novaPoshtaAPIKey := os.Getenv("NOVA_POSHTA_API_KEY")
	novaPoshtaURL := os.Getenv("NOVA_POSHTA_URL")
	if novaPoshtaURL == "" {
		novaPoshtaURL = "https://api.novaposhta.ua/v2.0/json/"
	}

	// Cloudinary Config
	cloudinaryURL := os.Getenv("CLOUDINARY_URL")
	if cloudinaryURL == "" {
		log.Println("Warning: CLOUDINARY_URL environment variable is not set. Media storage will not work.")
	}

	storeLogoURL := os.Getenv("STORE_LOGO_URL")
	if storeLogoURL == "" {
		// Fallback to the one we just uploaded if not set
		storeLogoURL = "https://res.cloudinary.com/dv94l5bvb/image/upload/v1776437405/aquawheel/system/logo.png"
	}

	apiHost := os.Getenv("API_HOST")

	// OTP Send Rate Limiting
	otpSendRateLimitStr := os.Getenv("OTP_SEND_RATE_LIMIT")
	otpSendRateLimit := 2 // Default to 2 requests
	if otpSendRateLimitStr != "" {
		if val, err := strconv.Atoi(otpSendRateLimitStr); err == nil {
			otpSendRateLimit = val
		}
	}

	otpSendRateIntervalStr := os.Getenv("OTP_SEND_RATE_INTERVAL")
	otpSendRateInterval := 3 * time.Minute // Default to 3 minutes
	if otpSendRateIntervalStr != "" {
		if val, err := time.ParseDuration(otpSendRateIntervalStr); err == nil {
			otpSendRateInterval = val
		}
	}

	emailRateLimitStr := os.Getenv("EMAIL_RATE_LIMIT")
	emailRateLimit := 2 // Default to 2
	if emailRateLimitStr != "" {
		if val, err := strconv.Atoi(emailRateLimitStr); err == nil {
			emailRateLimit = val
		}
	}

	emailRateIntervalStr := os.Getenv("EMAIL_RATE_INTERVAL")
	emailRateInterval := 3 * time.Minute // Default to 3 minutes
	if emailRateIntervalStr != "" {
		if val, err := time.ParseDuration(emailRateIntervalStr); err == nil {
			emailRateInterval = val
		}
	}

	trustedProxiesStr := os.Getenv("TRUSTED_PROXIES")
	var trustedProxies []string
	if trustedProxiesStr == "all" {
		trustedProxies = nil // nil means trust all proxies in Gin
	} else if trustedProxiesStr != "" {
		trustedProxies = strings.Split(trustedProxiesStr, ",")
	} else {
		trustedProxies = []string{"127.0.0.1"} // Default to localhost
	}
	// Badge Config
	badgeNewDays := getEnvInt("BADGE_NEW_DAYS", 7)
	badgeBestSellerThreshold := getEnvInt("BADGE_BESTSELLER_THRESHOLD", 5)

	// Order / Payment config
	adminNotificationEmail := os.Getenv("ADMIN_NOTIFICATION_EMAIL")
	if adminNotificationEmail == "" {
		log.Println("Warning: ADMIN_NOTIFICATION_EMAIL is not set. Admin order notifications will be skipped.")
	}
	liqPayPublicKey := os.Getenv("LIQPAY_PUBLIC_KEY")
	liqPayPrivateKey := os.Getenv("LIQPAY_PRIVATE_KEY")
	liqPayCallbackURL := os.Getenv("LIQPAY_CALLBACK_URL")
	stripeSecretKey := os.Getenv("STRIPE_SECRET_KEY")
	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	redsysMerchantCode := os.Getenv("REDSYS_MERCHANT_CODE")
	redsysTerminal := os.Getenv("REDSYS_TERMINAL")
	redsysSecretKey := os.Getenv("REDSYS_SECRET_KEY")
	redsysCallbackURL := os.Getenv("REDSYS_CALLBACK_URL")
	redsysCurrencyCode := os.Getenv("REDSYS_CURRENCY_CODE")
	if liqPayPublicKey == "" || liqPayPrivateKey == "" {
		log.Println("Warning: LIQPAY_PUBLIC_KEY / LIQPAY_PRIVATE_KEY not set. Payment will not work.")
	}

	// Nova Poshta Sender Config (for TTN creation)
	npSenderRef := os.Getenv("NP_SENDER_REF")
	npSenderCityRef := os.Getenv("NP_SENDER_CITY_REF")
	npSenderAddressRef := os.Getenv("NP_SENDER_ADDRESS_REF")
	npContactSenderRef := os.Getenv("NP_CONTACT_SENDER_REF")
	npSenderPhone := os.Getenv("NP_SENDER_PHONE")

	npTrackingIntervalMinutesStr := os.Getenv("NP_TRACKING_INTERVAL_MINUTES")
	npTrackingIntervalMinutes := 30
	if npTrackingIntervalMinutesStr != "" {
		if val, err := strconv.Atoi(npTrackingIntervalMinutesStr); err == nil && val > 0 {
			npTrackingIntervalMinutes = val
		}
	}
	if npSenderRef == "" || npContactSenderRef == "" {
		log.Println("Warning: NP_SENDER_REF / NP_CONTACT_SENDER_REF not set. TTN creation will not work.")
	}

	managerBaseURL := os.Getenv("MANAGER_BASE_URL")
	if managerBaseURL == "" {
		managerBaseURL = apiHost // fallback
	}

	// Vodafone OBM (SMS) Config
	obmBaseURL := os.Getenv("OBM_BASE_URL")
	if obmBaseURL == "" {
		obmBaseURL = "https://a2p.vodafone.ua"
	}
	obmTokenPath := os.Getenv("OBM_TOKEN_PATH")
	if obmTokenPath == "" {
		obmTokenPath = "/uaa/oauth/token"
	}
	obmBasicAuthHeader := os.Getenv("OBM_BASIC_AUTH_HEADER")
	if obmBasicAuthHeader == "" {
		obmBasicAuthHeader = "Basic d2ViYXBwOndlYmFwcA=="
	}
	obmUsername := os.Getenv("OBM_USERNAME")
	obmPassword := os.Getenv("OBM_PASSWORD")
	obmSenderID := getEnvInt("OBM_SENDER_ID", 0)
	obmDistributionID := os.Getenv("OBM_DISTRIBUTION_ID")
	obmValidityMinutes := os.Getenv("OBM_VALIDITY_MINUTES")
	if obmValidityMinutes == "" {
		obmValidityMinutes = "2"
	}
	// Перевірка статусу доставки увімкнена за замовчуванням (вимкнути: OBM_STATUS_CHECK=false)
	obmStatusCheck := os.Getenv("OBM_STATUS_CHECK") != "false"
	if obmUsername == "" || obmPassword == "" || obmSenderID == 0 {
		log.Println("Warning: OBM_USERNAME / OBM_PASSWORD / OBM_SENDER_ID not set. SMS verification will fall back to console (mock) sender.")
	}

	phoneCodeTTLStr := os.Getenv("PHONE_CODE_TTL")
	phoneCodeTTL := 2 * time.Minute
	if phoneCodeTTLStr != "" {
		if val, err := time.ParseDuration(phoneCodeTTLStr); err == nil {
			phoneCodeTTL = val
		}
	}

	corsOriginsStr := os.Getenv("CORS_ALLOW_ORIGINS")
	if err := validateStartupSecurity(appEnv, jwtSecret, corsOriginsStr); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}
	var corsAllowOrigins []string
	if corsOriginsStr != "" {
		// Remove spaces and split by comma
		corsOriginsStr = strings.ReplaceAll(corsOriginsStr, " ", "")
		corsAllowOrigins = strings.Split(corsOriginsStr, ",")
	} else {
		corsAllowOrigins = []string{"http://localhost:3000"}
	}

	storeCode := getEnvString("STORE_CODE", "default-store")
	storeName := getEnvString("STORE_NAME", "ecommerce-core store")
	defaultLocale := getEnvString("DEFAULT_LOCALE", "uk")
	supportedLocales := getEnvList("SUPPORTED_LOCALES", []string{"uk", "en"})
	fallbackLocale := getEnvString("FALLBACK_LOCALE", defaultLocale)
	currency := getEnvString("CURRENCY", "UAH")
	priceScale := getEnvInt("PRICE_SCALE", 2)
	taxMode := getEnvString("TAX_MODE", "none")
	vatRate := getEnvInt("VAT_RATE", 0)
	paymentProviders := getEnvList("PAYMENT_PROVIDERS", nil)
	paymentDefault := getEnvString("PAYMENT_DEFAULT", "")
	shippingProviders := getEnvList("SHIPPING_PROVIDERS", nil)
	shippingDefault := getEnvString("SHIPPING_DEFAULT", "")
	inventoryMode := getEnvString("INVENTORY_MODE", "internal")
	enabledModules := getEnvList("ENABLED_MODULES", nil)
	checkoutAllowGuest := getEnvBool("CHECKOUT_ALLOW_GUEST", true)
	checkoutRequirePhone := getEnvBool("CHECKOUT_REQUIRE_PHONE", true)
	defaultWarehouseID := getEnvString("DEFAULT_WAREHOUSE_ID", "")
	checkoutReservationTTL := 15 * time.Minute
	if value := os.Getenv("CHECKOUT_RESERVATION_TTL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			log.Printf("Warning: invalid CHECKOUT_RESERVATION_TTL, using 15m")
		} else {
			checkoutReservationTTL = parsed
		}
	}
	apiRateLimitPerMin := getEnvInt("API_RATE_LIMIT_PER_MINUTE", 100)
	sensitiveRatePerMin := getEnvInt("SENSITIVE_RATE_LIMIT_PER_MINUTE", 10)
	if apiRateLimitPerMin <= 0 || sensitiveRatePerMin <= 0 {
		log.Fatal("Fatal: API_RATE_LIMIT_PER_MINUTE and SENSITIVE_RATE_LIMIT_PER_MINUTE must be positive")
	}

	return &Config{
		Port:                      port,
		DBURL:                     dbURL,
		JWTSecret:                 jwtSecret,
		AccessTokenDuration:       accessTokenDuration,
		RefreshTokenDuration:      refreshTokenDuration,
		FrontendURL:               frontendURL,
		GoogleClientID:            googleClientID,
		MaxSessions:               maxSessions,
		Env:                       appEnv,
		CookieSecure:              appEnv == "production",
		APIRateLimitPerMin:        apiRateLimitPerMin,
		SensitiveRatePerMin:       sensitiveRatePerMin,
		RequestTimeout:            requestTimeout,
		SMTPHost:                  smtpHost,
		SMTPPort:                  smtpPort,
		SMTPUser:                  smtpUser,
		SMTPPassword:              smtpPassword,
		SMTPFrom:                  smtpFrom,
		SendGridAPIKey:            sendGridAPIKey,
		EmailFrom:                 emailFrom,
		NovaPoshtaAPIKey:          novaPoshtaAPIKey,
		NovaPoshtaURL:             novaPoshtaURL,
		CloudinaryURL:             cloudinaryURL,
		StoreLogoURL:              storeLogoURL,
		APIHost:                   apiHost,
		OTPSendRateLimit:          otpSendRateLimit,
		OTPSendRateInterval:       otpSendRateInterval,
		EmailRateLimit:            emailRateLimit,
		EmailRateInterval:         emailRateInterval,
		TrustedProxies:            trustedProxies,
		BadgeNewDays:              badgeNewDays,
		BadgeBestSellerThreshold:  badgeBestSellerThreshold,
		AdminNotificationEmail:    adminNotificationEmail,
		LiqPayPublicKey:           liqPayPublicKey,
		LiqPayPrivateKey:          liqPayPrivateKey,
		LiqPayCallbackURL:         liqPayCallbackURL,
		StripeSecretKey:           stripeSecretKey,
		StripeWebhookSecret:       stripeWebhookSecret,
		RedsysMerchantCode:        redsysMerchantCode,
		RedsysTerminal:            redsysTerminal,
		RedsysSecretKey:           redsysSecretKey,
		RedsysCallbackURL:         redsysCallbackURL,
		RedsysCurrencyCode:        redsysCurrencyCode,
		NPSenderRef:               npSenderRef,
		NPSenderCityRef:           npSenderCityRef,
		NPSenderAddressRef:        npSenderAddressRef,
		NPContactSenderRef:        npContactSenderRef,
		NPSenderPhone:             npSenderPhone,
		NPTrackingIntervalMinutes: npTrackingIntervalMinutes,
		ManagerBaseURL:            managerBaseURL,
		OBMBaseURL:                obmBaseURL,
		OBMTokenPath:              obmTokenPath,
		OBMBasicAuthHeader:        obmBasicAuthHeader,
		OBMUsername:               obmUsername,
		OBMPassword:               obmPassword,
		OBMSenderID:               obmSenderID,
		OBMDistributionID:         obmDistributionID,
		OBMValidityMinutes:        obmValidityMinutes,
		OBMStatusCheck:            obmStatusCheck,
		PhoneCodeTTL:              phoneCodeTTL,
		CORSAllowOrigins:          corsAllowOrigins,
		StoreCode:                 storeCode,
		StoreName:                 storeName,
		DefaultLocale:             defaultLocale,
		SupportedLocales:          supportedLocales,
		FallbackLocale:            fallbackLocale,
		Currency:                  currency,
		PriceScale:                priceScale,
		TaxMode:                   taxMode,
		VATRate:                   vatRate,
		PaymentProviders:          paymentProviders,
		PaymentDefault:            paymentDefault,
		ShippingProviders:         shippingProviders,
		ShippingDefault:           shippingDefault,
		InventoryMode:             inventoryMode,
		EnabledModules:            enabledModules,
		CheckoutAllowGuest:        checkoutAllowGuest,
		CheckoutRequirePhone:      checkoutRequirePhone,
		CheckoutReservationTTL:    checkoutReservationTTL,
		DefaultWarehouseID:        defaultWarehouseID,
	}
}

// validateStartupSecurity rejects insecure deployment defaults before a
// database connection or HTTP listener can be created.
func validateStartupSecurity(appEnv, jwtSecret, corsOrigins string) error {
	if jwtSecret == "" || jwtSecret == insecureDefaultJWTSecret {
		return fmt.Errorf("JWT_SECRET must be explicitly configured and must not use the insecure default")
	}
	if appEnv == "production" && strings.TrimSpace(corsOrigins) == "" {
		return fmt.Errorf("CORS_ALLOW_ORIGINS is required when APP_ENV=production")
	}
	return nil
}

func getEnvInt(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

func getEnvString(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func getEnvList(key string, defaultValue []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func getEnvBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Printf("Warning: Invalid %s: %v. Using default: %t", key, err, defaultValue)
		return defaultValue
	}
	return parsed
}

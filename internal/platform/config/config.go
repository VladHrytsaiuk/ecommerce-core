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
	GoogleClientSecret   string
	GoogleRedirectURI    string
	OAuthAttemptTTL      time.Duration
	ProfilePolicyJSON    string
	ComparisonMaxItems   int
	MaxSessions          int
	Env                  string
	CookieSecure         bool
	APIRateLimitPerMin   int
	SensitiveRatePerMin  int

	// HTTP server
	// RequestTimeout — верхня межа тривалості обробки HTTP-запиту (context deadline).
	// Має бути достатньою для повільних операцій (напр., завантаження кількох зображень у Cloudinary).
	RequestTimeout time.Duration
	ManagementAddr string
	OTelEnabled    bool
	OTelEndpoint   string

	// SMTP (Email)
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPTLSMode  string

	// Transactional Notifications. The encryption key is a base64-encoded
	// 32-byte AES-256 key and is validated by the Notifications composition.
	NotificationEmailProvider string
	NotificationEncryptionKey string

	// Redis is an optional infrastructure capability used only for cache and
	// distributed rate limiting. Financial workflows continue to use PostgreSQL.
	RedisEnabled bool
	RedisURL     string

	// Search is an opt-in external read projection. PostgreSQL remains the
	// source of truth; these credentials are validated only when enabled.
	SearchURL         string
	SearchMasterKey   string
	SearchIndexPrefix string
	ReportsTimezone   string

	// Media is an optional object-storage capability. S3-compatible providers
	// share credentials; Cloudinary uses its own provider URL.
	MediaProvider          string
	MediaS3Bucket          string
	MediaS3Region          string
	MediaS3Endpoint        string
	MediaS3AccessKeyID     string
	MediaS3SecretAccessKey string
	MediaS3PublicBaseURL   string
	MediaR2PublicBaseURL   string
	MediaS3UsePathStyle    bool
	MediaCloudinaryURL     string

	// SendGrid (Email)
	SendGridAPIKey string
	EmailFrom      string

	// Nova Poshta (Shipping)
	NovaPoshtaAPIKey string
	NovaPoshtaURL    string

	// DHL Express (MyDHL API)
	DHLExpressBaseURL       string
	DHLExpressUsername      string
	DHLExpressPassword      string
	DHLExpressAccountNumber string
	DHLExpressProductCode   string
	DHLExpressSenderName    string
	DHLExpressSenderPhone   string
	DHLExpressSenderCountry string
	DHLExpressSenderPostal  string
	DHLExpressSenderCity    string
	DHLExpressSenderLine1   string
	DHLExpressPackageLength float64
	DHLExpressPackageWidth  float64
	DHLExpressPackageHeight float64

	// Store logo is optional branding for legacy transactional email templates.
	StoreLogoURL string

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
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	googleRedirectURI := os.Getenv("GOOGLE_REDIRECT_URI")
	profilePolicyJSON := os.Getenv("PROFILE_POLICY_JSON")
	oauthAttemptTTL := 10 * time.Minute
	if value := os.Getenv("OAUTH_ATTEMPT_TTL"); value != "" {
		parsed, parseErr := time.ParseDuration(value)
		if parseErr != nil || parsed <= 0 || parsed > time.Hour {
			log.Fatal("Fatal: OAUTH_ATTEMPT_TTL must be between 1ns and 1h")
		}
		oauthAttemptTTL = parsed
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
	smtpTLSMode := strings.ToLower(getEnvString("SMTP_TLS_MODE", "starttls"))
	notificationEmailProvider := strings.ToLower(getEnvString("NOTIFICATION_EMAIL_PROVIDER", "mock"))
	notificationEncryptionKey := strings.TrimSpace(os.Getenv("NOTIFICATION_ENCRYPTION_KEY"))
	redisEnabled := getEnvBool("REDIS_ENABLED", false)
	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	searchURL := strings.TrimSpace(os.Getenv("SEARCH_URL"))
	searchMasterKey := strings.TrimSpace(os.Getenv("SEARCH_MASTER_KEY"))
	searchIndexPrefix := getEnvString("SEARCH_INDEX_PREFIX", "ecommerce")
	reportsTimezone := strings.TrimSpace(getEnvString("REPORTS_TIMEZONE", "UTC"))
	mediaProvider := strings.ToLower(getEnvString("MEDIA_PROVIDER", "s3"))
	mediaS3Bucket := strings.TrimSpace(os.Getenv("MEDIA_S3_BUCKET"))
	mediaS3Region := strings.TrimSpace(getEnvString("MEDIA_S3_REGION", "us-east-1"))
	mediaS3Endpoint := strings.TrimSpace(os.Getenv("MEDIA_S3_ENDPOINT"))
	mediaS3AccessKeyID := strings.TrimSpace(os.Getenv("MEDIA_S3_ACCESS_KEY_ID"))
	mediaS3SecretAccessKey := strings.TrimSpace(os.Getenv("MEDIA_S3_SECRET_ACCESS_KEY"))
	mediaS3PublicBaseURL := strings.TrimSpace(os.Getenv("MEDIA_S3_PUBLIC_BASE_URL"))
	mediaR2PublicBaseURL := strings.TrimSpace(os.Getenv("MEDIA_R2_PUBLIC_BASE_URL"))
	mediaS3UsePathStyle := getEnvBool("MEDIA_S3_USE_PATH_STYLE", false)
	mediaCloudinaryURL := strings.TrimSpace(os.Getenv("MEDIA_CLOUDINARY_URL"))
	if redisEnabled && redisURL == "" {
		log.Fatal("Fatal: REDIS_URL is required when REDIS_ENABLED=true")
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

	// DHL Express (MyDHL API). The default deliberately targets DHL's test
	// environment; production deployments must opt in to the live URL.
	dhlExpressBaseURL := getEnvString("DHL_EXPRESS_BASE_URL", "https://express.api.dhl.com/mydhlapi/test")
	dhlExpressUsername := os.Getenv("DHL_EXPRESS_USERNAME")
	dhlExpressPassword := os.Getenv("DHL_EXPRESS_PASSWORD")
	dhlExpressAccountNumber := os.Getenv("DHL_EXPRESS_ACCOUNT_NUMBER")
	dhlExpressProductCode := os.Getenv("DHL_EXPRESS_PRODUCT_CODE")
	dhlExpressSenderName := os.Getenv("DHL_EXPRESS_SENDER_NAME")
	dhlExpressSenderPhone := os.Getenv("DHL_EXPRESS_SENDER_PHONE")
	dhlExpressSenderCountry := os.Getenv("DHL_EXPRESS_SENDER_COUNTRY")
	dhlExpressSenderPostal := os.Getenv("DHL_EXPRESS_SENDER_POSTAL")
	dhlExpressSenderCity := os.Getenv("DHL_EXPRESS_SENDER_CITY")
	dhlExpressSenderLine1 := os.Getenv("DHL_EXPRESS_SENDER_LINE1")
	dhlExpressPackageLength := getEnvFloat("DHL_EXPRESS_PACKAGE_LENGTH_CM", 0)
	dhlExpressPackageWidth := getEnvFloat("DHL_EXPRESS_PACKAGE_WIDTH_CM", 0)
	dhlExpressPackageHeight := getEnvFloat("DHL_EXPRESS_PACKAGE_HEIGHT_CM", 0)

	storeLogoURL := strings.TrimSpace(os.Getenv("STORE_LOGO_URL"))

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
	comparisonMaxItems := 5
	if raw := strings.TrimSpace(os.Getenv("COMPARISON_MAX_ITEMS")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			log.Fatal("Fatal: COMPARISON_MAX_ITEMS must be an integer")
		}
		comparisonMaxItems = parsed
	}
	apiRateLimitPerMin := getEnvInt("API_RATE_LIMIT_PER_MINUTE", 100)
	managementAddr := getEnvString("MANAGEMENT_ADDR", "127.0.0.1:9090")
	otelEnabled := getEnvBool("OTEL_ENABLED", false)
	otelEndpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
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
		GoogleClientSecret:        googleClientSecret,
		GoogleRedirectURI:         googleRedirectURI,
		OAuthAttemptTTL:           oauthAttemptTTL,
		ProfilePolicyJSON:         profilePolicyJSON,
		ComparisonMaxItems:        comparisonMaxItems,
		MaxSessions:               maxSessions,
		Env:                       appEnv,
		CookieSecure:              appEnv == "production",
		APIRateLimitPerMin:        apiRateLimitPerMin,
		SensitiveRatePerMin:       sensitiveRatePerMin,
		RequestTimeout:            requestTimeout,
		ManagementAddr:            managementAddr,
		OTelEnabled:               otelEnabled,
		OTelEndpoint:              otelEndpoint,
		SMTPHost:                  smtpHost,
		SMTPPort:                  smtpPort,
		SMTPUser:                  smtpUser,
		SMTPPassword:              smtpPassword,
		SMTPFrom:                  smtpFrom,
		SMTPTLSMode:               smtpTLSMode,
		NotificationEmailProvider: notificationEmailProvider,
		NotificationEncryptionKey: notificationEncryptionKey,
		RedisEnabled:              redisEnabled,
		RedisURL:                  redisURL,
		SearchURL:                 searchURL,
		ReportsTimezone:           reportsTimezone,
		SearchMasterKey:           searchMasterKey,
		SearchIndexPrefix:         searchIndexPrefix,
		MediaProvider:             mediaProvider,
		MediaS3Bucket:             mediaS3Bucket,
		MediaS3Region:             mediaS3Region,
		MediaS3Endpoint:           mediaS3Endpoint,
		MediaS3AccessKeyID:        mediaS3AccessKeyID,
		MediaS3SecretAccessKey:    mediaS3SecretAccessKey,
		MediaS3PublicBaseURL:      mediaS3PublicBaseURL,
		MediaR2PublicBaseURL:      mediaR2PublicBaseURL,
		MediaS3UsePathStyle:       mediaS3UsePathStyle,
		MediaCloudinaryURL:        mediaCloudinaryURL,
		SendGridAPIKey:            sendGridAPIKey,
		EmailFrom:                 emailFrom,
		NovaPoshtaAPIKey:          novaPoshtaAPIKey,
		NovaPoshtaURL:             novaPoshtaURL,
		DHLExpressBaseURL:         dhlExpressBaseURL,
		DHLExpressUsername:        dhlExpressUsername,
		DHLExpressPassword:        dhlExpressPassword,
		DHLExpressAccountNumber:   dhlExpressAccountNumber,
		DHLExpressProductCode:     dhlExpressProductCode,
		DHLExpressSenderName:      dhlExpressSenderName,
		DHLExpressSenderPhone:     dhlExpressSenderPhone,
		DHLExpressSenderCountry:   dhlExpressSenderCountry,
		DHLExpressSenderPostal:    dhlExpressSenderPostal,
		DHLExpressSenderCity:      dhlExpressSenderCity,
		DHLExpressSenderLine1:     dhlExpressSenderLine1,
		DHLExpressPackageLength:   dhlExpressPackageLength,
		DHLExpressPackageWidth:    dhlExpressPackageWidth,
		DHLExpressPackageHeight:   dhlExpressPackageHeight,
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

func getEnvFloat(key string, defaultValue float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("Warning: Invalid %s: %v. Using default: %g", key, err, defaultValue)
		return defaultValue
	}
	return parsed
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

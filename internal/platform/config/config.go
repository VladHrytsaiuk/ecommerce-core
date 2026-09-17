// Package config loads and holds every environment-provided setting this
// application starts from. A value that cannot be honoured fails the boot here
// rather than at the call site that needed it.
package config

import (
	"fmt"
	"log"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const insecureDefaultJWTSecret = "very_secret_key_change_me_in_prod"

// maxAccessTokenDuration caps how long a revoked role stays effective. One hour
// is generous for an access token whose refresh lifetime is measured in days.
const maxAccessTokenDuration = time.Hour

// Config is the whole environment-provided configuration, already parsed and
// validated.
type Config struct {
	Port                 string
	DBURL                string
	DBPool               DBPool
	JWTSecret            string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	FrontendURL          string
	// AuthMethods lists the enabled sign-in methods; see app.AuthMethod.
	AuthMethods        []string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	OAuthAttemptTTL    time.Duration
	// OneTimeCodeTTL is how long a one-time code works, emailed or texted.
	OneTimeCodeTTL      time.Duration
	ProfilePolicyJSON   string
	ComparisonMaxItems  int
	MaxSessions         int
	Env                 string
	CookieSecure        bool
	APIRateLimitPerMin  int
	WebhookRatePerMin   int
	SensitiveRatePerMin int

	// HTTP server
	// RequestTimeout is the upper bound on one HTTP request, applied as a
	// context deadline. It has to cover the slowest legitimate request — an
	// image upload that reaches object storage — without letting a stuck one
	// hold a connection indefinitely.
	RequestTimeout time.Duration
	// ShutdownTimeout bounds the HTTP drain on SIGTERM.
	ShutdownTimeout time.Duration
	ManagementAddr  string
	OTelEnabled     bool
	OTelEndpoint    string

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
	SearchURL                 string
	SearchMasterKey           string
	SearchIndexPrefix         string
	ReportsTimezone           string
	OutboxDoneRetention       time.Duration
	OutboxArchiveRetention    time.Duration
	NotificationSentRetention time.Duration
	NotificationDeadRetention time.Duration
	OutboxRetentionInterval   time.Duration

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

	// Video is intentionally separate from Media: image object stores do not
	// provide adaptive streaming. Cloudflare Stream credentials are consumed
	// only when the opt-in video module is enabled.
	VideoProvider                  string
	CloudflareStreamAccountID      string
	CloudflareStreamAPIToken       string
	CloudflareStreamWebhookSecret  string
	CloudflareStreamAllowedOrigins []string
	// Signed playback keeps a Stream video private: the storefront receives a
	// short-lived token rather than a permanent provider URL, so a copied link
	// stops working instead of becoming an open mirror of paid content.
	CloudflareStreamCustomerCode  string
	CloudflareStreamSigningKeyID  string
	CloudflareStreamSigningKeyPEM string
	VideoPlaybackTTL              time.Duration

	// Sync export. The ERP receives a signed POST; anything able to accept one
	// needs no bespoke adapter in this core.
	SyncExportURL     string
	SyncExportSecret  string
	SyncExportTimeout time.Duration
	SyncDispatchLease time.Duration
	SyncRetryDelay    time.Duration
	SyncMaxAttempts   int

	EmailFrom string

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

	// Trusted Proxies
	TrustedProxies []string
	// Dynamic Badges
	BadgeNewDays             int
	BadgeBestSellerThreshold int

	// Order / Payment
	AdminNotificationEmail   string
	LiqPayPublicKey          string
	LiqPayPrivateKey         string
	LiqPayCallbackURL        string
	MonobankToken            string
	MonobankWebhookPublicKey string
	MonobankAPIURL           string
	MonobankWebhookURL       string
	StripeSecretKey          string
	StripeWebhookSecret      string
	RedsysMerchantCode       string
	RedsysTerminal           string
	RedsysSecretKey          string
	RedsysCallbackURL        string
	RedsysCurrencyCode       string

	// Nova Poshta Sender (for TTN creation)
	NPSenderRef               string
	NPSenderCityRef           string
	NPSenderAddressRef        string
	NPContactSenderRef        string
	NPSenderPhone             string
	NPTrackingIntervalMinutes int

	// Manager
	ManagerBaseURL string

	// SMS delivers phone sign-in codes; see app.AuthMethodPhoneCode.
	SMSProvider            string
	SMSAllowedCountryCodes []string
	SMSHourlyLimit         int
	SMSCodeMessage         string
	VodafoneOBMBaseURL     string
	VodafoneOBMTokenPath   string
	VodafoneOBMBasicAuth   string
	VodafoneOBMUsername    string
	VodafoneOBMPassword    string
	VodafoneOBMSenderID    int
	CORSAllowOrigins       []string

	// Store configuration. These fields are consumed by internal/app during the
	// Strangler Fig migration; legacy services continue to use the fields above.
	StoreCode                     string
	StoreName                     string
	DefaultLocale                 string
	SupportedLocales              []string
	FallbackLocale                string
	Currency                      string
	PriceScale                    int
	TaxMode                       string
	VATRate                       int
	PaymentProviders              []string
	PaymentDefault                string
	ShippingProviders             []string
	ShippingDefault               string
	InventoryMode                 string
	EnabledModules                []string
	CheckoutAllowGuest            bool
	CheckoutRequirePhone          bool
	CheckoutRequireVerifiedEmail  bool
	CheckoutRequireVerifiedPhone  bool
	CheckoutRequiredProfileFields []string
	CheckoutReservationTTL        time.Duration
	// ReturnWindowDays configures the customer-facing RMA eligibility window.
	// It is consumed only when the optional returns module is enabled.
	ReturnWindowDays                     int
	AbandonedCartDelays                  []time.Duration
	AbandonedCartMaxReminders            int
	AbandonedCartRequireMarketingConsent bool
	AbandonedCartQuietHours              string
	DefaultWarehouseID                   string
}

// Load reads .env when present and returns a fully validated Config.
//
// A missing or unusable required value ends the process here, by design: a
// store that starts with a setting it cannot honour fails later, in front of a
// customer, instead of at boot in front of whoever deployed it.
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
	dbPool, err := parseDBPool(os.Getenv)
	if err != nil {
		log.Fatal("Fatal: " + err.Error())
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
	// Access tokens carry the role and cannot be revoked, so expiry is the only
	// thing that ends an administrator's privileges after a demotion. A long
	// lifetime therefore silently converts a permission change into a delay of
	// that length. Bound it rather than trusting the deployment to be sensible.
	if accessTokenDuration <= 0 || accessTokenDuration > maxAccessTokenDuration {
		log.Fatalf("Fatal: ACCESS_TOKEN_DURATION must be between 1ns and %s; it is the only bound on a revoked role", maxAccessTokenDuration)
	}

	refreshTokenDurationStr := os.Getenv("REFRESH_TOKEN_DURATION")
	if refreshTokenDurationStr == "" {
		refreshTokenDurationStr = "168h"
	}
	refreshTokenDuration, err := time.ParseDuration(refreshTokenDurationStr)
	if err != nil {
		log.Fatalf("Fatal: Invalid REFRESH_TOKEN_DURATION format: %v", err)
	}
	if err := validateRefreshTokenDuration(refreshTokenDuration, accessTokenDuration); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	authMethods := getEnvList("AUTH_METHODS", nil)
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

	oneTimeCodeTTL := getEnvDuration("ONE_TIME_CODE_TTL", DefaultOneTimeCodeTTL)
	if err := validateOneTimeCodeTTL(oneTimeCodeTTL); err != nil {
		log.Fatal("Fatal: " + err.Error())
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

	// 30s by default: enough for the slowest legitimate request, which is a
	// product create or update that carries several images through to object
	// storage.
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
	outboxDoneRetention := getEnvDuration("OUTBOX_DONE_RETENTION", 30*24*time.Hour)
	outboxRetentionInterval := getEnvDuration("OUTBOX_RETENTION_INTERVAL", time.Hour)
	if outboxDoneRetention <= 0 || outboxRetentionInterval <= 0 {
		log.Fatal("Fatal: OUTBOX_DONE_RETENTION and OUTBOX_RETENTION_INTERVAL must be positive durations")
	}
	// Archiving only moved a delivery between two tables; nothing ever emptied
	// the second one, so "retention" made the hot table small and the database
	// no smaller. The archive exists for forensics after the fact, which is a
	// question asked in weeks, not years.
	outboxArchiveRetention := getEnvDuration("OUTBOX_ARCHIVE_RETENTION", 90*24*time.Hour)
	if err := validateOutboxArchiveRetention(outboxArchiveRetention, outboxDoneRetention); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}
	// A sent job is evidence a message went out; a dead one is a message that
	// never did and an operator may still need to act on, so it is kept longer.
	notificationSentRetention := getEnvDuration("NOTIFICATION_SENT_RETENTION", 30*24*time.Hour)
	notificationDeadRetention := getEnvDuration("NOTIFICATION_DEAD_RETENTION", 90*24*time.Hour)
	if err := validateNotificationRetention(notificationSentRetention, notificationDeadRetention, outboxDoneRetention); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}
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
	videoProvider := strings.ToLower(getEnvString("VIDEO_PROVIDER", "cloudflare"))
	cloudflareStreamAccountID := strings.TrimSpace(os.Getenv("CLOUDFLARE_STREAM_ACCOUNT_ID"))
	cloudflareStreamAPIToken := strings.TrimSpace(os.Getenv("CLOUDFLARE_STREAM_API_TOKEN"))
	cloudflareStreamWebhookSecret := strings.TrimSpace(os.Getenv("CLOUDFLARE_STREAM_WEBHOOK_SECRET"))
	cloudflareStreamAllowedOrigins := getEnvList("CLOUDFLARE_STREAM_ALLOWED_ORIGINS", nil)
	cloudflareStreamCustomerCode := strings.TrimSpace(os.Getenv("CLOUDFLARE_STREAM_CUSTOMER_CODE"))
	cloudflareStreamSigningKeyID := strings.TrimSpace(os.Getenv("CLOUDFLARE_STREAM_SIGNING_KEY_ID"))
	cloudflareStreamSigningKeyPEM := strings.TrimSpace(os.Getenv("CLOUDFLARE_STREAM_SIGNING_KEY_PEM"))
	syncExportURL := strings.TrimSpace(os.Getenv("SYNC_EXPORT_URL"))
	syncExportSecret := strings.TrimSpace(os.Getenv("SYNC_EXPORT_SECRET"))
	syncExportTimeout := getEnvDuration("SYNC_EXPORT_TIMEOUT", 30*time.Second)
	syncDispatchLease := getEnvDuration("SYNC_DISPATCH_LEASE", time.Minute)
	syncRetryDelay := getEnvDuration("SYNC_RETRY_DELAY", time.Minute)
	syncMaxAttempts := getEnvInt("SYNC_MAX_ATTEMPTS", 10)
	// The lease has to outlast a slow ERP call, or a second dispatcher reclaims
	// the event while the first is still waiting on the same export.
	if syncExportTimeout <= 0 || syncDispatchLease <= syncExportTimeout {
		log.Fatal("Fatal: SYNC_DISPATCH_LEASE must be greater than SYNC_EXPORT_TIMEOUT")
	}
	videoPlaybackTTL := getEnvDuration("VIDEO_PLAYBACK_TTL", 4*time.Hour)
	// A playback token is the only thing standing between a storefront visitor
	// and an unrestricted copy of the video, so its lifetime is bounded here
	// rather than trusted to the deployment.
	if videoPlaybackTTL < time.Minute || videoPlaybackTTL > 24*time.Hour {
		log.Fatal("Fatal: VIDEO_PLAYBACK_TTL must be between 1m and 24h")
	}
	if redisEnabled && redisURL == "" {
		log.Fatal("Fatal: REDIS_URL is required when REDIS_ENABLED=true")
	}

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

	trustedProxies, err := parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		// Trusting arbitrary or malformed proxy hops lets an internet client
		// forge X-Forwarded-For and evade IP-scoped abuse controls. Refuse to
		// boot rather than silently falling back to an unsafe interpretation.
		log.Fatalf("Fatal: invalid TRUSTED_PROXIES: %v", err)
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
	monobankToken := os.Getenv("MONOBANK_TOKEN")
	monobankWebhookPublicKey := os.Getenv("MONOBANK_WEBHOOK_PUBLIC_KEY")
	monobankAPIURL := strings.TrimSpace(os.Getenv("MONOBANK_API_URL"))
	if monobankAPIURL == "" {
		monobankAPIURL = "https://api.monobank.ua"
	}
	monobankWebhookURL := os.Getenv("MONOBANK_WEBHOOK_URL")
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

	smsProvider := strings.ToLower(strings.TrimSpace(os.Getenv("SMS_PROVIDER")))
	smsAllowedCountryCodes := getEnvList("SMS_ALLOWED_COUNTRY_CODES", nil)
	smsHourlyLimit := getEnvInt("SMS_HOURLY_LIMIT", DefaultSMSHourlyLimit)
	smsCodeMessage := os.Getenv("SMS_CODE_MESSAGE")
	if strings.TrimSpace(smsCodeMessage) == "" {
		smsCodeMessage = DefaultSMSCodeMessage
	}
	if err := validateSMSProvider(appEnv, smsProvider, containsFold(authMethods, "phone_code")); err != nil {
		log.Fatal("Fatal: " + err.Error())
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
	// Canonicalised here, once, because every consumer downstream compares
	// module names case-insensitively: app.NewModuleSet lowercases, the CLI
	// uses EqualFold, and the migration runner matches lowercase directory
	// names. containsModule below did not, so ENABLED_MODULES=Notifications
	// enabled the module and silently skipped both guards that validate it.
	// Normalising at the boundary removes the divergence rather than patching
	// one comparison.
	enabledModules := NormalizeModules(getEnvList("ENABLED_MODULES", nil))
	checkoutAllowGuest := getEnvBool("CHECKOUT_ALLOW_GUEST", true)
	checkoutRequirePhone := getEnvBool("CHECKOUT_REQUIRE_PHONE", true)
	checkoutRequireVerifiedEmail := getEnvBool("CHECKOUT_REQUIRE_VERIFIED_EMAIL", false)
	checkoutRequireVerifiedPhone := getEnvBool("CHECKOUT_REQUIRE_VERIFIED_PHONE", false)
	checkoutRequiredProfileFields := getEnvList("CHECKOUT_REQUIRED_PROFILE_FIELDS", nil)
	returnWindowDays := 14
	if raw := strings.TrimSpace(os.Getenv("RETURN_WINDOW")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			log.Fatal("Fatal: RETURN_WINDOW must be an integer number of days")
		}
		returnWindowDays = parsed
	}
	abandonedCartDelays := make([]time.Duration, 0)
	for _, raw := range getEnvList("ABANDONED_CART_DELAYS", nil) {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			log.Fatal("Fatal: ABANDONED_CART_DELAYS must be comma-separated positive Go durations")
		}
		abandonedCartDelays = append(abandonedCartDelays, value)
	}
	abandonedCartMaxReminders := getEnvInt("ABANDONED_CART_MAX_REMINDERS", len(abandonedCartDelays))
	abandonedCartRequireMarketingConsent := getEnvBool("ABANDONED_CART_REQUIRE_MARKETING_CONSENT", true)
	abandonedCartQuietHours := strings.TrimSpace(os.Getenv("ABANDONED_CART_QUIET_HOURS"))
	if containsModule(enabledModules, "abandoned_cart") && (len(abandonedCartDelays) == 0 || abandonedCartMaxReminders < 1 || abandonedCartMaxReminders > len(abandonedCartDelays)) {
		log.Fatal("Fatal: abandoned_cart requires delays and a valid ABANDONED_CART_MAX_REMINDERS")
	}
	if err := validateNotificationProvider(appEnv, notificationEmailProvider, containsModule(enabledModules, "notifications")); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}
	if err := validateMarketingLinks(appEnv, frontendURL, containsModule(enabledModules, "abandoned_cart")); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}
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
	// The drain has to outlast a request's own budget, or SIGTERM cuts off work
	// the server itself said it would allow: it was five seconds against a
	// thirty-second request timeout, and the two numbers could drift silently
	// because nothing related them. The margin covers writing the last response
	// after the handler returns.
	shutdownTimeout := getEnvDuration("SHUTDOWN_TIMEOUT", requestTimeout+shutdownDrainMargin)
	if err := validateShutdownTimeout(shutdownTimeout, requestTimeout); err != nil {
		log.Fatal("Fatal: " + err.Error())
	}
	managementAddr := getEnvString("MANAGEMENT_ADDR", "127.0.0.1:9090")
	otelEnabled := getEnvBool("OTEL_ENABLED", false)
	otelEndpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	sensitiveRatePerMin := getEnvInt("SENSITIVE_RATE_LIMIT_PER_MINUTE", 10)
	// Webhooks get their own budget. A payment provider calls from its own
	// small set of addresses, so every callback for the whole store shares one
	// per-IP bucket; the browsing limit throttled the integration hardest
	// exactly when it mattered most, working off a backlog after an outage.
	// Signature verification, not this number, is what keeps the endpoint safe.
	webhookRatePerMin := getEnvInt("WEBHOOK_RATE_LIMIT_PER_MINUTE", 600)
	if apiRateLimitPerMin <= 0 || sensitiveRatePerMin <= 0 || webhookRatePerMin <= 0 {
		log.Fatal("Fatal: API_RATE_LIMIT_PER_MINUTE, SENSITIVE_RATE_LIMIT_PER_MINUTE and WEBHOOK_RATE_LIMIT_PER_MINUTE must be positive")
	}

	return &Config{
		Port:                                 port,
		DBURL:                                dbURL,
		DBPool:                               dbPool,
		JWTSecret:                            jwtSecret,
		AccessTokenDuration:                  accessTokenDuration,
		RefreshTokenDuration:                 refreshTokenDuration,
		FrontendURL:                          frontendURL,
		AuthMethods:                          authMethods,
		GoogleClientID:                       googleClientID,
		GoogleClientSecret:                   googleClientSecret,
		GoogleRedirectURI:                    googleRedirectURI,
		OAuthAttemptTTL:                      oauthAttemptTTL,
		OneTimeCodeTTL:                       oneTimeCodeTTL,
		ProfilePolicyJSON:                    profilePolicyJSON,
		ComparisonMaxItems:                   comparisonMaxItems,
		MaxSessions:                          maxSessions,
		Env:                                  appEnv,
		CookieSecure:                         appEnv == "production",
		APIRateLimitPerMin:                   apiRateLimitPerMin,
		WebhookRatePerMin:                    webhookRatePerMin,
		SensitiveRatePerMin:                  sensitiveRatePerMin,
		RequestTimeout:                       requestTimeout,
		ShutdownTimeout:                      shutdownTimeout,
		ManagementAddr:                       managementAddr,
		OTelEnabled:                          otelEnabled,
		OTelEndpoint:                         otelEndpoint,
		SMTPHost:                             smtpHost,
		SMTPPort:                             smtpPort,
		SMTPUser:                             smtpUser,
		SMTPPassword:                         smtpPassword,
		SMTPFrom:                             smtpFrom,
		SMTPTLSMode:                          smtpTLSMode,
		NotificationEmailProvider:            notificationEmailProvider,
		NotificationEncryptionKey:            notificationEncryptionKey,
		RedisEnabled:                         redisEnabled,
		RedisURL:                             redisURL,
		SearchURL:                            searchURL,
		ReportsTimezone:                      reportsTimezone,
		OutboxDoneRetention:                  outboxDoneRetention,
		OutboxArchiveRetention:               outboxArchiveRetention,
		NotificationSentRetention:            notificationSentRetention,
		NotificationDeadRetention:            notificationDeadRetention,
		OutboxRetentionInterval:              outboxRetentionInterval,
		SearchMasterKey:                      searchMasterKey,
		SearchIndexPrefix:                    searchIndexPrefix,
		MediaProvider:                        mediaProvider,
		MediaS3Bucket:                        mediaS3Bucket,
		MediaS3Region:                        mediaS3Region,
		MediaS3Endpoint:                      mediaS3Endpoint,
		MediaS3AccessKeyID:                   mediaS3AccessKeyID,
		MediaS3SecretAccessKey:               mediaS3SecretAccessKey,
		MediaS3PublicBaseURL:                 mediaS3PublicBaseURL,
		MediaR2PublicBaseURL:                 mediaR2PublicBaseURL,
		MediaS3UsePathStyle:                  mediaS3UsePathStyle,
		MediaCloudinaryURL:                   mediaCloudinaryURL,
		VideoProvider:                        videoProvider,
		CloudflareStreamAccountID:            cloudflareStreamAccountID,
		CloudflareStreamAPIToken:             cloudflareStreamAPIToken,
		CloudflareStreamWebhookSecret:        cloudflareStreamWebhookSecret,
		CloudflareStreamAllowedOrigins:       cloudflareStreamAllowedOrigins,
		CloudflareStreamCustomerCode:         cloudflareStreamCustomerCode,
		CloudflareStreamSigningKeyID:         cloudflareStreamSigningKeyID,
		CloudflareStreamSigningKeyPEM:        cloudflareStreamSigningKeyPEM,
		VideoPlaybackTTL:                     videoPlaybackTTL,
		SyncExportURL:                        syncExportURL,
		SyncExportSecret:                     syncExportSecret,
		SyncExportTimeout:                    syncExportTimeout,
		SyncDispatchLease:                    syncDispatchLease,
		SyncRetryDelay:                       syncRetryDelay,
		SyncMaxAttempts:                      syncMaxAttempts,
		EmailFrom:                            emailFrom,
		NovaPoshtaAPIKey:                     novaPoshtaAPIKey,
		NovaPoshtaURL:                        novaPoshtaURL,
		DHLExpressBaseURL:                    dhlExpressBaseURL,
		DHLExpressUsername:                   dhlExpressUsername,
		DHLExpressPassword:                   dhlExpressPassword,
		DHLExpressAccountNumber:              dhlExpressAccountNumber,
		DHLExpressProductCode:                dhlExpressProductCode,
		DHLExpressSenderName:                 dhlExpressSenderName,
		DHLExpressSenderPhone:                dhlExpressSenderPhone,
		DHLExpressSenderCountry:              dhlExpressSenderCountry,
		DHLExpressSenderPostal:               dhlExpressSenderPostal,
		DHLExpressSenderCity:                 dhlExpressSenderCity,
		DHLExpressSenderLine1:                dhlExpressSenderLine1,
		DHLExpressPackageLength:              dhlExpressPackageLength,
		DHLExpressPackageWidth:               dhlExpressPackageWidth,
		DHLExpressPackageHeight:              dhlExpressPackageHeight,
		StoreLogoURL:                         storeLogoURL,
		APIHost:                              apiHost,
		TrustedProxies:                       trustedProxies,
		BadgeNewDays:                         badgeNewDays,
		BadgeBestSellerThreshold:             badgeBestSellerThreshold,
		AdminNotificationEmail:               adminNotificationEmail,
		LiqPayPublicKey:                      liqPayPublicKey,
		LiqPayPrivateKey:                     liqPayPrivateKey,
		LiqPayCallbackURL:                    liqPayCallbackURL,
		MonobankToken:                        monobankToken,
		MonobankWebhookPublicKey:             monobankWebhookPublicKey,
		MonobankAPIURL:                       monobankAPIURL,
		MonobankWebhookURL:                   monobankWebhookURL,
		StripeSecretKey:                      stripeSecretKey,
		StripeWebhookSecret:                  stripeWebhookSecret,
		RedsysMerchantCode:                   redsysMerchantCode,
		RedsysTerminal:                       redsysTerminal,
		RedsysSecretKey:                      redsysSecretKey,
		RedsysCallbackURL:                    redsysCallbackURL,
		RedsysCurrencyCode:                   redsysCurrencyCode,
		NPSenderRef:                          npSenderRef,
		NPSenderCityRef:                      npSenderCityRef,
		NPSenderAddressRef:                   npSenderAddressRef,
		NPContactSenderRef:                   npContactSenderRef,
		NPSenderPhone:                        npSenderPhone,
		NPTrackingIntervalMinutes:            npTrackingIntervalMinutes,
		ManagerBaseURL:                       managerBaseURL,
		SMSProvider:                          smsProvider,
		SMSAllowedCountryCodes:               smsAllowedCountryCodes,
		SMSHourlyLimit:                       smsHourlyLimit,
		SMSCodeMessage:                       smsCodeMessage,
		VodafoneOBMBaseURL:                   os.Getenv("VODAFONE_OBM_BASE_URL"),
		VodafoneOBMTokenPath:                 os.Getenv("VODAFONE_OBM_TOKEN_PATH"),
		VodafoneOBMBasicAuth:                 os.Getenv("VODAFONE_OBM_BASIC_AUTH"),
		VodafoneOBMUsername:                  os.Getenv("VODAFONE_OBM_USERNAME"),
		VodafoneOBMPassword:                  os.Getenv("VODAFONE_OBM_PASSWORD"),
		VodafoneOBMSenderID:                  getEnvInt("VODAFONE_OBM_SENDER_ID", 0),
		CORSAllowOrigins:                     corsAllowOrigins,
		StoreCode:                            storeCode,
		StoreName:                            storeName,
		DefaultLocale:                        defaultLocale,
		SupportedLocales:                     supportedLocales,
		FallbackLocale:                       fallbackLocale,
		Currency:                             currency,
		PriceScale:                           priceScale,
		TaxMode:                              taxMode,
		VATRate:                              vatRate,
		PaymentProviders:                     paymentProviders,
		PaymentDefault:                       paymentDefault,
		ShippingProviders:                    shippingProviders,
		ShippingDefault:                      shippingDefault,
		InventoryMode:                        inventoryMode,
		EnabledModules:                       enabledModules,
		CheckoutAllowGuest:                   checkoutAllowGuest,
		CheckoutRequirePhone:                 checkoutRequirePhone,
		CheckoutRequireVerifiedEmail:         checkoutRequireVerifiedEmail,
		CheckoutRequireVerifiedPhone:         checkoutRequireVerifiedPhone,
		CheckoutRequiredProfileFields:        checkoutRequiredProfileFields,
		CheckoutReservationTTL:               checkoutReservationTTL,
		ReturnWindowDays:                     returnWindowDays,
		AbandonedCartDelays:                  abandonedCartDelays,
		AbandonedCartMaxReminders:            abandonedCartMaxReminders,
		AbandonedCartRequireMarketingConsent: abandonedCartRequireMarketingConsent,
		AbandonedCartQuietHours:              abandonedCartQuietHours,
		DefaultWarehouseID:                   defaultWarehouseID,
	}
}

// parseTrustedProxies accepts only explicit IPs or CIDR prefixes supported by
// Gin. Keeping parsing separate from LoadConfig makes the security policy
// testable without mutating process environment or invoking log.Fatal.
func parseTrustedProxies(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"127.0.0.1"}, nil
	}
	if strings.EqualFold(raw, "all") {
		return nil, fmt.Errorf("value %q is forbidden; configure explicit ingress proxy CIDRs", raw)
	}

	parts := strings.Split(raw, ",")
	proxies := make([]string, 0, len(parts))
	for _, part := range parts {
		proxy := strings.TrimSpace(part)
		if proxy == "" {
			return nil, fmt.Errorf("contains an empty proxy entry")
		}
		if _, err := netip.ParseAddr(proxy); err == nil {
			proxies = append(proxies, proxy)
			continue
		}
		if _, err := netip.ParsePrefix(proxy); err == nil {
			proxies = append(proxies, proxy)
			continue
		}
		return nil, fmt.Errorf("%q is neither an IP address nor a CIDR prefix", proxy)
	}
	return proxies, nil
}

// validateNotificationProvider refuses a production store that would silently
// throw its mail away.
//
// The mock sender writes to memory and the log and returns a successful
// receipt, so the job is marked sent and nothing anywhere reports a problem: a
// store on the default would record every order confirmation, support reply and
// refund notice as delivered while none of them left the building. It is the
// default precisely because it needs no credentials, which is exactly what
// makes it easy to inherit into production.
//
// Separate from validateStartupSecurity, and for the same reason that one
// exists: a rule worth enforcing is worth testing without invoking log.Fatal.
func validateNotificationProvider(appEnv, provider string, notificationsEnabled bool) error {
	if appEnv != "production" || !notificationsEnabled {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(provider)) == "mock" {
		return fmt.Errorf("NOTIFICATION_EMAIL_PROVIDER must not be \"mock\" when APP_ENV=production and the notifications module is enabled: it discards mail and reports it as sent")
	}
	return nil
}

// DefaultSMSHourlyLimit caps how many texts the whole store sends an hour when
// SMS_HOURLY_LIMIT is not set. Every text costs money, and a sign-in form that
// sends one to any number is what SMS pumping fraud looks for.
const DefaultSMSHourlyLimit = 100

// DefaultSMSCodeMessage is the text of a sign-in code SMS when SMS_CODE_MESSAGE
// is not set. {code} and {minutes} are replaced.
const DefaultSMSCodeMessage = "Your sign-in code: {code}. It works for {minutes} min. Do not share it."

// validateSMSProvider refuses a production store whose phone codes would go to
// the mock sender, which writes them to the log instead of a phone: anyone
// reading the log could sign in as any customer.
func validateSMSProvider(appEnv, provider string, phoneCodesEnabled bool) error {
	if appEnv != "production" || !phoneCodesEnabled {
		return nil
	}
	if provider == "mock" {
		return fmt.Errorf("SMS_PROVIDER must not be \"mock\" when APP_ENV=production and AUTH_METHODS includes phone_code: it writes sign-in codes to the log")
	}
	return nil
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

// shutdownDrainMargin is how much longer than a request's own budget the drain
// runs, so a handler that uses all of it can still have its response written.
const shutdownDrainMargin = 5 * time.Second

// validateShutdownTimeout keeps the drain from cutting off a request the server
// had already promised to serve.
//
// The two values are related, so the default is derived rather than typed
// twice; an operator who overrides it still cannot set it below the budget the
// timeout middleware hands each request.
func validateShutdownTimeout(shutdown, request time.Duration) error {
	if shutdown <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be a positive duration")
	}
	if shutdown < request {
		return fmt.Errorf("SHUTDOWN_TIMEOUT (%s) must be at least HTTP_REQUEST_TIMEOUT (%s), or SIGTERM cuts off requests the server is still allowed to be serving", shutdown, request)
	}
	return nil
}

// validateRefreshTokenDuration bounds how long a sign-in lasts. It was parsed and
// then used by nothing, so any value was accepted; now that it decides when a
// customer has to sign in again, a value that ends the sign-in before its first
// access token expires is refused.
func validateRefreshTokenDuration(refresh, access time.Duration) error {
	if refresh <= 0 {
		return fmt.Errorf("REFRESH_TOKEN_DURATION must be a positive duration")
	}
	if refresh < access {
		return fmt.Errorf("REFRESH_TOKEN_DURATION (%s) must be at least ACCESS_TOKEN_DURATION (%s)", refresh, access)
	}
	return nil
}

// DefaultOneTimeCodeTTL is how long a code works when ONE_TIME_CODE_TTL is not
// set. A Config built in code rather than by Load leaves OneTimeCodeTTL zero,
// which means the same.
const DefaultOneTimeCodeTTL = 10 * time.Minute

// validateOneTimeCodeTTL bounds how long a code works: long enough to switch to
// the mailbox or the messages and back, short enough that a code read over someone's
// shoulder is soon useless.
func validateOneTimeCodeTTL(ttl time.Duration) error {
	if ttl < time.Minute || ttl > 30*time.Minute {
		return fmt.Errorf("ONE_TIME_CODE_TTL (%s) must be between 1m and 30m", ttl)
	}
	return nil
}

// validateOutboxArchiveRetention keeps the two windows in the only order that
// makes sense: a delivery reaches the archive after OUTBOX_DONE_RETENTION, so
// an archive window shorter than that would delete rows the moment they arrive
// and leave no forensic trail at all.
func validateOutboxArchiveRetention(archive, outboxDone time.Duration) error {
	if archive <= 0 {
		return fmt.Errorf("OUTBOX_ARCHIVE_RETENTION must be a positive duration")
	}
	if archive < outboxDone {
		return fmt.Errorf("OUTBOX_ARCHIVE_RETENTION (%s) must be at least OUTBOX_DONE_RETENTION (%s), or a delivery would be deleted as soon as it is archived", archive, outboxDone)
	}
	return nil
}

// validateNotificationRetention keeps the sent window from outliving the outbox.
//
// A notification job is what stops a redelivered outbox event from sending the
// same message twice: the handler finds the existing job and returns. If sent
// jobs are purged sooner than completed deliveries are archived, a delivery
// replayed inside that gap finds no job, creates a new one, and the customer
// receives a second copy of an email they already have.
func validateNotificationRetention(sent, dead, outboxDone time.Duration) error {
	if sent <= 0 || dead <= 0 {
		return fmt.Errorf("NOTIFICATION_SENT_RETENTION and NOTIFICATION_DEAD_RETENTION must be positive durations")
	}
	if sent < outboxDone {
		return fmt.Errorf("NOTIFICATION_SENT_RETENTION (%s) must be at least OUTBOX_DONE_RETENTION (%s), or a replayed delivery could send a duplicate email", sent, outboxDone)
	}
	return nil
}

// validateMarketingLinks refuses a production store whose marketing mail would
// carry an opt-out link nobody outside the server can open.
//
// FRONTEND_URL defaults to http://localhost:3000, which is right for the
// storefront redirect it was originally for and wrong for a URL that leaves in
// somebody's inbox: a store that never set it would mail an unsubscribe link
// pointing at the recipient's own machine. The address a recipient cannot reach
// is the same as no way to unsubscribe at all, which is what the signed link
// exists to fix.
//
// Only where marketing is actually sent, and only in production, so a
// development store keeps working with the default.
func validateMarketingLinks(appEnv, frontendURL string, marketingEnabled bool) error {
	if appEnv != "production" || !marketingEnabled {
		return nil
	}
	host, err := absoluteURLHost(frontendURL)
	if err != nil {
		return fmt.Errorf("FRONTEND_URL must be the store's absolute public URL when APP_ENV=production and abandoned_cart is enabled, because unsubscribe links are built from it: %w", err)
	}
	if isLoopbackHost(host) {
		return fmt.Errorf("FRONTEND_URL points at %q, so unsubscribe links in marketing email would be unopenable; set it to the store's public URL", host)
	}
	return nil
}

// absoluteURLHost returns the host of an absolute http(s) URL, rejecting
// anything a mail client could not follow.
func absoluteURLHost(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%q is not a URL", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%q has no http or https scheme", raw)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("%q has no host", raw)
	}
	return parsed.Hostname(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
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

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("Warning: Invalid %s: %v. Using default: %s", key, err, defaultValue)
		return defaultValue
	}
	return parsed
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

// NormalizeModules is the single definition of a module name's canonical form:
// trimmed and lower-case, matching the Module constants, the migration
// directory names, and what every consumer already compares against.
//
// It is exported so the layers above compare against this rule rather than
// restate it. Three places used to decide whether a module was enabled and one
// of them folded case differently, which is how ENABLED_MODULES=Notifications
// came to enable a module whose configuration guards then never ran.
func NormalizeModules(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if module := strings.ToLower(strings.TrimSpace(value)); module != "" {
			normalized = append(normalized, module)
		}
	}
	return normalized
}

// containsModule reports whether a canonical module name is enabled. It takes
// the list as normalizeModules left it, so it compares like with like.
func containsModule(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
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

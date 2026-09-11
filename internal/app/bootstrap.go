// Package app contains the clean-slate Composition Root.
package app

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	abandonedReaders "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/adapter/readers"
	abandonedApp "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/application"
	abandonedPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/repository/postgres"
	googleAuthAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/auth/google"
	deliveryWorkflowAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/delivery/orderworkflow"
	syncHTTPExport "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/sync/httpexport"
	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	availabilityIdentity "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/adapter/identity"
	availabilityApp "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/application"
	availabilityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	availabilityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/repository/postgres"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	badgesPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/repository/postgres"
	badgesService "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/service"
	cartApp "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/application"
	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	cartPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/repository/postgres"
	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	catalogPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/repository/postgres"
	checkoutConsent "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/adapter/consent"
	checkoutApp "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/application"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	checkoutPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/repository/postgres"
	comparisonDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
	comparisonPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/repository/postgres"
	comparisonService "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/service"
	consentOrders "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/adapter/orders"
	consentApp "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	consentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/repository/postgres"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
	localeApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/application"
	localePostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/repository/postgres"
	orderWorkflowApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/application"
	orderWorkflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	deliveryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/application"
	deliveryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/application"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	identityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/repository/postgres"
	identityService "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/service"
	inventoryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/application"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	inventoryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/repository/postgres"
	mediaApp "github.com/VladHrytsaiuk/ecommerce-core/internal/media/application"
	mediaPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/media/repository/postgres"
	notificationsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/application"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	ordersApp "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/application"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	ordersPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/repository/postgres"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/management"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	workflowPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/orderworkflow"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	promosApp "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
	promosPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/repository/postgres"
	reportsOrderAnalytics "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/adapter/orderanalytics"
	reportsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application"
	reportsProjectors "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application/projectors"
	reportsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
	reportsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/repository/postgres"
	returnsFinance "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/adapter/finance"
	returnsInventory "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/adapter/inventory"
	returnsOrderSnapshot "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/adapter/ordersnapshot"
	returnsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/application"
	returnsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
	returnsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/repository/postgres"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	reviewsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/repository/postgres"
	reviewsService "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/service"
	searchCatalog "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/catalog"
	searchMeili "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/meilisearch"
	searchApp "github.com/VladHrytsaiuk/ecommerce-core/internal/search/application"
	searchDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
	seoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/repository/postgres"
	seoService "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/service"
	supportIdentity "github.com/VladHrytsaiuk/ecommerce-core/internal/support/adapter/identity"
	supportApp "github.com/VladHrytsaiuk/ecommerce-core/internal/support/application"
	supportPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/support/repository/postgres"
	syncApp "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/application"
	syncPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/repository/postgres"
	videoCloudflare "github.com/VladHrytsaiuk/ecommerce-core/internal/video/adapter/cloudflare"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
	videoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/video/repository/postgres"
	wishlistDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	wishlistPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/repository/postgres"
	wishlistService "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/service"
)

// Application exposes only services that belong to the active clean-slate graph.
type Application struct {
	Config                    *config.Config
	StoreConfig               StoreConfig
	TokenMaker                token.Maker
	CatalogCategoryService    catalogDomain.CategoryService
	CatalogProductService     catalogDomain.ProductService
	CatalogVariantService     catalogDomain.VariantService
	CartService               cartDomain.Service
	CheckoutService           checkoutDomain.Service
	CheckoutContactCapture    checkoutDomain.ContactCaptureService
	CheckoutRecovery          *checkoutApp.RecoveryService
	CheckoutExpiry            *checkoutApp.ExpiryService
	OrderWorkflowService      orderWorkflowDomain.Service
	InventoryService          inventoryDomain.Service
	InventoryAvailability     catalogDomain.VariantAvailabilityReader
	InventoryCleanup          *inventoryApp.Cleanup
	OrderService              ordersDomain.Service
	IdentityAuthService       identityDomain.AuthService
	IdentityProfileService    identityDomain.ProfileService
	CustomerProfileService    identityDomain.CustomerProfileService
	WishlistService           wishlistDomain.Service
	ComparisonService         comparisonDomain.Service
	ReviewsService            reviewsDomain.Service
	SEOService                seoDomain.Service
	BadgesService             badgesDomain.Service
	PaymentGateways           *paymentsApp.Registry
	PaymentWebhookService     *paymentsApp.WebhookService
	DeliveryCarriers          *deliveryApp.Registry
	DeliveryLocations         *deliveryApp.LocationService
	DeliveryDispatcher        *deliveryApp.Dispatcher
	DeliveryTracker           *deliveryApp.Tracker
	OutboxWorker              *eventsApp.OutboxWorker
	AdminAuditOutboxWorker    *eventsApp.OutboxWorker
	SearchOutboxWorker        *eventsApp.OutboxWorker
	MediaOutboxWorker         *eventsApp.OutboxWorker
	ReportsOutboxWorker       *eventsApp.OutboxWorker
	OutboxRetention           *eventsApp.RetentionWorker
	SyncDispatcher            *syncApp.Dispatcher
	MediaOrphanCleanup        *mediaApp.OrphanCleanupWorker
	SearchService             searchDomain.SearchService
	MediaUploadService        *mediaApp.UploadService
	VideoUploadService        *videoApp.DirectUploadService
	VideoPlacementFacade      *videoApp.PlacementAdminFacade
	VideoWebhookService       *videoApp.WebhookService
	VideoOrphanCleanup        *videoApp.OrphanCleanupWorker
	VideoStorefront           *videoApp.StorefrontService
	ReportsQueryService       reportsDomain.QueryService
	ReportsRebuilder          *reportsApp.ReportsRebuilder
	ReturnService             *returnsApp.ReturnService
	ReturnsOutboxWorker       *eventsApp.OutboxWorker
	NotificationWorker        *notificationsApp.DurableWorker
	AvailabilityService       *availabilityApp.Service
	AvailabilityOutboxWorker  *eventsApp.OutboxWorker
	AbandonedCartWorker       *abandonedApp.Worker
	AbandonedCartOutboxWorker *eventsApp.OutboxWorker
	SupportService            *supportApp.Service
	ConsentService            *consentApp.Service
	AdminAuthorizer           adminDomain.Authorizer
	PromosAdminFacade         *adminApp.PromosAdminFacade
	CatalogAdminFacade        *adminApp.CatalogAdminFacade
	ContentAdminFacade        *adminApp.ContentAdminFacade
	OrdersAdminFacade         *adminApp.OrdersAdminFacade
	TaxPolicy                 tax.Calculator
	HTTP                      HTTPDependencies
	Management                *management.Server
	resourceCloser            io.Closer
	telemetryShutdown         func(context.Context) error
	workerMu                  sync.Mutex
	workerCancel              context.CancelFunc
	workerWG                  sync.WaitGroup
}

type HTTPDependencies struct {
	Recovery              gin.HandlerFunc
	Observability         gin.HandlerFunc
	Timeout               gin.HandlerFunc
	RequestLogging        gin.HandlerFunc
	CORS                  gin.HandlerFunc
	SecurityHeaders       gin.HandlerFunc
	ErrorRenderer         *apiresponse.ErrorRenderer
	Swagger               gin.HandlerFunc
	Health                gin.HandlerFunc
	LocaleMiddleware      gin.HandlerFunc
	OptionalAuth          gin.HandlerFunc
	LoginRateLimit        gin.HandlerFunc
	APIRateLimit          gin.HandlerFunc
	RequestBodyLimit      gin.HandlerFunc
	MediaRequestBodyLimit gin.HandlerFunc
}

// Bootstrap is the sole Composition Root for the active clean-slate modules.
func Bootstrap(cfg *config.Config, storeConfig StoreConfig, db *gorm.DB, tokenMaker token.Maker) (*Application, error) {
	if err := storeConfig.Validate(); err != nil {
		return nil, err
	}
	modules := storeConfig.Modules()
	// Support ticket creation is an abuse-sensitive public endpoint. Its limiter
	// must be shared and durable across replicas; a per-process fallback would
	// silently multiply the quota during a rollout or restart.
	if modules.Has(ModuleSupport) && !cfg.RedisEnabled {
		return nil, fmt.Errorf("support requires REDIS_ENABLED=true for distributed anti-spam enforcement")
	}
	taxPolicy, err := tax.NewPolicy(tax.Mode(storeConfig.TaxMode), storeConfig.VATRate)
	if err != nil {
		return nil, err
	}
	paymentGateways, err := newPaymentRegistry(cfg, storeConfig)
	if err != nil {
		return nil, err
	}
	deliveryCarriers, err := newDeliveryRegistry(cfg, storeConfig)
	if err != nil {
		return nil, err
	}
	if err := localeApp.NewService(localePostgres.NewRepository(db)).Synchronize(context.Background(), storeConfig.SupportedLocales, storeConfig.DefaultLocale); err != nil {
		return nil, err
	}

	platform, err := newPlatformRuntime(cfg, storeConfig)
	if err != nil {
		return nil, err
	}
	// Every failure below returns without committing, which releases the Redis
	// pool and the trace exporter rather than leaking them into a process that
	// is exiting. Success transfers ownership to the Application.
	defer platform.rollback()
	cacheService := platform.Cache
	loginLimiter := platform.LoginLimiter

	adminEnabled := modules.Has(ModuleAdmin)
	var adminAuthorizer adminDomain.Authorizer
	if adminEnabled {
		authorizer, authorizerErr := adminApp.NewAuthorizer(adminPostgres.NewRepository(db), cacheService, 0)
		if authorizerErr != nil {
			return nil, fmt.Errorf("configure admin RBAC: %w", authorizerErr)
		}
		adminAuthorizer = authorizer.WithLogger(logger.Log)
	}

	corsMiddleware, err := middleware.NewCORS(cfg.CORSAllowOrigins)
	if err != nil {
		return nil, fmt.Errorf("configure CORS: %w", err)
	}
	errorRenderer := apiresponse.NewErrorRenderer(logger.Log)
	httpDependencies := HTTPDependencies{
		Recovery:      middleware.PanicRecovery(errorRenderer),
		Observability: observability.Middleware(),
		Timeout:       middleware.TimeoutMiddleware(cfg.RequestTimeout),
		RequestLogging: func(c *gin.Context) {
			logger.WithContext(c.Request.Context()).Infow("Inbound Request", "method", c.Request.Method, "path", c.Request.URL.Path, "ip", c.ClientIP())
			c.Next()
		},
		CORS:             corsMiddleware,
		SecurityHeaders:  middleware.SecurityHeaders(),
		ErrorRenderer:    errorRenderer,
		Swagger:          ginSwagger.WrapHandler(swaggerFiles.Handler),
		Health:           func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) },
		LocaleMiddleware: middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: storeConfig.DefaultLocale, FallbackLocale: storeConfig.FallbackLocale, SupportedLocales: storeConfig.SupportedLocales}),
		OptionalAuth:     middleware.OptionalAuthMiddleware(tokenMaker),
		LoginRateLimit:   middleware.LoginRateLimitMiddleware(loginLimiter, cfg.JWTSecret),
		APIRateLimit:     middleware.RateLimitByIP(loginLimiter, "api:v1", cfg.APIRateLimitPerMin, time.Minute, cfg.JWTSecret, errorRenderer),
		RequestBodyLimit: middleware.MaxRequestBodyBytes(1<<20, errorRenderer),
		// Includes multipart framing while ReadImagePart independently enforces
		// a strict 15 MiB limit for the file itself.
		MediaRequestBodyLimit: middleware.MaxRequestBodyBytes(16<<20, errorRenderer),
	}

	variantService := catalogApp.NewVariantService(catalogPostgres.NewVariantRepository(db), storeConfig.SupportedLocales, storeConfig.Currency).
		WithFallbackLocale(storeConfig.FallbackLocale)
	productOptionsService := catalogApp.NewProductOptionsService(catalogPostgres.NewOptionsRepository(db), storeConfig.Currency)
	inventoryRepository := inventoryPostgres.NewRepository(db)
	inventoryService := inventoryApp.NewService(inventoryMode(storeConfig.InventoryMode), inventoryRepository)
	notificationsEnabled := modules.Has(ModuleNotifications)
	availabilityEnabled := modules.Has(ModuleAvailability)
	abandonedCartEnabled := modules.Has(ModuleAbandonedCart)
	supportEnabled := modules.Has(ModuleSupport)
	consentEnabled := modules.Has(ModuleConsent)
	reportsEnabled := modules.Has(ModuleReports)
	returnsEnabled := modules.Has(ModuleReturns)
	videoEnabled := modules.Has(ModuleVideo)
	workflowEventRoutes := orderWorkflowEventRoutes(notificationsEnabled, reportsEnabled, returnsEnabled)
	var availabilityService *availabilityApp.Service
	var availabilityOutboxWorker *eventsApp.OutboxWorker
	if availabilityEnabled {
		notificationRepository := notificationsPostgres.NewRepository(db)
		repository := availabilityPostgres.NewRepository(db)
		availabilityService = availabilityApp.NewService(repository, availabilityIdentity.NewEmailReader(db))
		handler := availabilityApp.NewHandler(repository, notificationRepository, func(ctx context.Context, fn func(context.Context) error) error {
			return adminPostgres.NewTransactionManager(db).WithinTransaction(ctx, fn)
		})
		availabilityOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), availabilityDomain.ConsumerAvailabilityNotifications, time.Minute, logger.Log, handler).WithTracer(observability.NewOutboxTracer())
		inventoryService.WithAvailabilityPublisher(eventsPostgres.NewPublisher(availabilityDomain.ConsumerAvailabilityNotifications))
	}
	var supportService *supportApp.Service
	if supportEnabled {
		notificationRepository := notificationsPostgres.NewRepository(db)
		supportService = supportApp.NewService(supportPostgres.NewRepository(db), supportApp.NewSpamProtector(loginLimiter), supportIdentity.NewEmailReader(db)).WithAdminWorkflow(adminPostgres.NewTransactionManager(db), notificationRepository)
	}
	var consentService *consentApp.Service
	if consentEnabled {
		consentService = consentApp.NewService(consentPostgres.NewRepository(db), consentOrders.NewActivityReader(db)).WithAdminWorkflow(adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher())
	}
	// The workflow repository is PostgreSQL infrastructure. It is deliberately
	// outside core so core/application code does not depend on an Orders or
	// Inventory repository implementation.
	workflowEventPublisher := eventsPostgres.NewTopicPublisher(workflowEventRoutes)
	workflowRepository := workflowPostgres.NewRepository(db, modules.Has(ModuleSync)).
		WithEventPublisher(workflowEventPublisher).
		// The status lifecycle is already durably recorded in
		// order_status_history. Persist its public event separately until a
		// module explicitly subscribes, so payment/report workers never claim
		// unrelated status deliveries.
		WithStatusEventPublisher(workflowEventPublisher)
	var operationalWorkflowPolicy *ordersApp.WorkflowService
	if modules.Has(ModuleOrders) {
		operationalWorkflowPolicy, err = ordersApp.NewWorkflowService(ordersPostgres.NewWorkflowRepository(db))
		if err != nil {
			return nil, fmt.Errorf("configure order workflow policy: %w", err)
		}
		workflowRepository.WithOperationalTransitionPolicy(operationalWorkflowPolicy)
	}
	basePriceCalculator, err := checkoutDomain.NewCheckoutPriceCalculator(taxPolicy)
	if err != nil {
		return nil, err
	}
	var priceCalculator checkoutDomain.PriceCalculator = basePriceCalculator
	var promosRepository *promosPostgres.Repository
	if modules.Has(ModulePromos) {
		promosRepository = promosPostgres.NewRepository(db)
		workflowRepository.WithTransactionHook(promosApp.NewWorkflowHook(promosRepository))
		priceCalculator = promosApp.NewPromoCalculatorDecorator(priceCalculator, promosRepository)
	}
	orderWorkflowService := orderWorkflowApp.NewService(workflowRepository)
	var deliveryOrderTransitioner *deliveryWorkflowAdapter.Bridge
	if operationalWorkflowPolicy != nil {
		deliveryOrderTransitioner, err = deliveryWorkflowAdapter.NewBridge(orderWorkflowService)
		if err != nil {
			return nil, fmt.Errorf("configure delivery order workflow bridge: %w", err)
		}
	}
	paymentWebhookService := paymentsApp.NewWebhookService(paymentGateways, paymentsPostgres.NewWebhookEventStore(db), orderWorkflowService)
	var enabledWishlist wishlistDomain.Service
	if modules.Has(ModuleWishlist) {
		enabledWishlist = wishlistService.New(wishlistPostgres.NewRepository(db))
	}
	var enabledComparison comparisonDomain.Service
	if modules.Has(ModuleComparison) {
		enabledComparison = comparisonService.New(comparisonPostgres.NewRepository(db), storeConfig.ComparisonMaxItems)
	}
	productService := catalogApp.NewProductService(catalogPostgres.NewProductRepository(db), storeConfig.SupportedLocales)
	searchEnabled := modules.Has(ModuleSearch)
	var enabledSEO seoDomain.Service
	if modules.Has(ModuleSEO) {
		repository := seoPostgres.NewRepository(db)
		enabledSEO = seoService.New(repository)
		productService.WithSEOReader(repository)
	}
	var enabledBadges badgesDomain.Service
	if modules.Has(ModuleBadges) {
		repository := badgesPostgres.NewRepository(db)
		enabledBadges = badgesService.New(repository)
		productService.WithBadgeReader(repository)
	}
	var enabledReviews reviewsDomain.Service
	if modules.Has(ModuleReviews) {
		repository := reviewsPostgres.NewRepository(db)
		enabledReviews = reviewsService.New(repository)
		productService.WithRatingReader(repository)
	}
	oauthProviders := make([]identityDomain.OAuthProvider, 0, 1)
	if storeConfig.GoogleOAuth != nil {
		googleProvider, err := googleAuthAdapter.New(googleAuthAdapter.Config{
			ClientID:            storeConfig.GoogleOAuth.ClientID,
			ClientSecret:        storeConfig.GoogleOAuth.ClientSecret,
			AllowedRedirectURIs: []string{storeConfig.GoogleOAuth.RedirectURI},
		})
		if err != nil {
			return nil, err
		}
		oauthProviders = append(oauthProviders, googleProvider)
	}
	identityAuthService := identityService.NewAuthService(
		identityPostgres.NewUserRepository(db),
		identityPostgres.NewOAuthIdentityRepository(db),
		identityPostgres.NewOAuthAttemptStore(db),
		identityPostgres.NewAuthTransaction(db),
		identityService.NewOAuthProviderRegistry(oauthProviders...),
		tokenMaker,
		cfg.AccessTokenDuration,
		storeConfig.OAuthAttemptTTL,
	)
	if observer := newUserLoginObserver(newWishlistLoginObserver(enabledWishlist), newComparisonLoginObserver(enabledComparison)); observer != nil {
		identityAuthService.WithUserLoginObserver(observer)
	}
	var identityProfileService identityDomain.ProfileService
	var customerProfileService identityDomain.CustomerProfileService
	if modules.Has(ModuleCustomers) {
		customerProfileService = identityApplication.NewCustomerProfileService(identityPostgres.NewCustomerProfileRepository(db))
	}
	if modules.Has(ModuleUserProfiles) && storeConfig.ProfilePolicy != nil {
		identityProfileService = identityService.NewProfileService(*storeConfig.ProfilePolicy, identityPostgres.NewProfileRepository(db))
	}
	outboxHandlers := make([]eventsApp.Consumer, 0, 1)
	var notificationWorker *notificationsApp.DurableWorker
	if notificationsEnabled {
		cipher, err := encryption.NewAESGCM(cfg.NotificationEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("configure notifications encryption: %w", err)
		}
		sender, err := newNotificationEmailSender(cfg)
		if err != nil {
			return nil, fmt.Errorf("configure notifications email sender: %w", err)
		}
		repository := notificationsPostgres.NewRepository(db)
		renderer := notificationsApp.NewTemplateRenderer(repository, storeConfig.DefaultLocale)
		outboxHandlers = append(outboxHandlers, notificationsApp.NewOrderPaidEventHandler(repository, cipher, renderer, sender))
		notificationWorker = notificationsApp.NewDurableWorker(repository, renderer, sender)
	}
	var promosAdminFacade *adminApp.PromosAdminFacade
	var catalogAdminFacade *adminApp.CatalogAdminFacade
	var contentAdminFacade *adminApp.ContentAdminFacade
	var ordersAdminFacade *adminApp.OrdersAdminFacade
	var adminAuditOutboxWorker *eventsApp.OutboxWorker
	var searchOutboxWorker *eventsApp.OutboxWorker
	var mediaOutboxWorker *eventsApp.OutboxWorker
	var reportsOutboxWorker *eventsApp.OutboxWorker
	outboxRetention, err := eventsApp.NewRetentionWorker(eventsPostgres.NewRetentionStore(db), cfg.OutboxDoneRetention, 1000, logger.Log)
	if err != nil {
		return nil, fmt.Errorf("configure outbox retention: %w", err)
	}
	var syncDispatcher *syncApp.Dispatcher
	if modules.Has(ModuleSync) {
		exporter, exportErr := syncHTTPExport.New(syncHTTPExport.Config{
			Endpoint: cfg.SyncExportURL, Secret: cfg.SyncExportSecret, Timeout: cfg.SyncExportTimeout,
		})
		if exportErr != nil {
			return nil, fmt.Errorf("configure sync order export: %w", exportErr)
		}
		syncDispatcher = syncApp.NewDispatcher(syncPostgres.NewOutboxStore(db), exporter, cfg.SyncRetryDelay, cfg.SyncDispatchLease, cfg.SyncMaxAttempts)
	}
	var mediaOrphanCleanup *mediaApp.OrphanCleanupWorker
	var productEventPublisher eventsDomain.TransactionalEventPublisher
	var searchService searchDomain.SearchService
	var mediaUploadService *mediaApp.UploadService
	var mediaCatalogReader *mediaApp.CatalogReader
	var reportsQueryService reportsDomain.QueryService
	var reportsRebuilder *reportsApp.ReportsRebuilder
	var returnService *returnsApp.ReturnService
	var returnsOutboxWorker *eventsApp.OutboxWorker
	var abandonedCartWorker *abandonedApp.Worker
	var abandonedCartOutboxWorker *eventsApp.OutboxWorker
	if reportsEnabled {
		repository := reportsPostgres.NewRepository(db)
		reportsQueryService, err = reportsApp.NewQueryService(repository, cfg.ReportsTimezone)
		if err != nil {
			return nil, fmt.Errorf("configure reports queries: %w", err)
		}
		snapshotProvider := reportsOrderAnalytics.NewProvider(db)
		reportsRebuilder, err = reportsApp.NewReportsRebuilder(repository, repository, snapshotProvider, cfg.ReportsTimezone)
		if err != nil {
			return nil, fmt.Errorf("configure reports rebuilder: %w", err)
		}
		reportsRebuilder = reportsRebuilder.WithLogger(logger.Log)
		handler, err := reportsProjectors.NewDailySalesProjector(repository, repository, snapshotProvider, cfg.ReportsTimezone)
		if err != nil {
			return nil, fmt.Errorf("configure reports daily sales projector: %w", err)
		}
		refundHandler, err := reportsProjectors.NewRefundProjector(repository, repository, snapshotProvider, cfg.ReportsTimezone)
		if err != nil {
			return nil, fmt.Errorf("configure reports refund projector: %w", err)
		}
		cartFunnel, err := reportsProjectors.NewFunnelProjector(repository, repository, eventsDomain.TopicCartCreated, cfg.ReportsTimezone)
		if err != nil {
			return nil, fmt.Errorf("configure reports cart funnel projector: %w", err)
		}
		checkoutFunnel, err := reportsProjectors.NewFunnelProjector(repository, repository, eventsDomain.TopicCheckoutStarted, cfg.ReportsTimezone)
		if err != nil {
			return nil, fmt.Errorf("configure reports checkout funnel projector: %w", err)
		}
		reportsOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerReportsProjection, time.Minute, logger.Log, handler, refundHandler, cartFunnel, checkoutFunnel).WithTracer(observability.NewOutboxTracer())
	}
	if searchEnabled {
		searchStartupCtx, cancelSearchStartup := context.WithTimeout(context.Background(), searchMeili.DefaultTaskTimeout)
		index, err := searchMeili.New(searchStartupCtx, searchMeili.Config{
			URL: cfg.SearchURL, MasterKey: cfg.SearchMasterKey,
			IndexUID: cfg.SearchIndexPrefix + "_" + storeConfig.Code + "_products",
		})
		cancelSearchStartup()
		if err != nil {
			return nil, fmt.Errorf("configure search index: %w", err)
		}
		platform.adopt(index)
		handler, err := searchApp.NewProductChangedHandler(searchCatalog.NewSnapshotProvider(productService), index)
		if err != nil {
			return nil, fmt.Errorf("configure search indexer: %w", err)
		}
		searchService, err = searchApp.NewSearchService(index)
		if err != nil {
			return nil, fmt.Errorf("configure search service: %w", err)
		}
		searchOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerSearchIndexer, time.Minute, logger.Log, handler).WithTracer(observability.NewOutboxTracer())
		productEventPublisher = eventsPostgres.NewPublisher(eventsDomain.ConsumerSearchIndexer)
	}
	if adminEnabled {
		auditHandler := adminApp.NewAdminAuditEventHandler(adminPostgres.NewAuditRepository(db))
		adminAuditOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerAdminAudit, time.Minute, logger.Log, auditHandler).WithTracer(observability.NewOutboxTracer())
		if promosRepository != nil {
			promosAdminFacade, err = adminApp.NewPromosAdminFacade(adminAuthorizer, promosApp.NewAdminService(promosRepository), adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
			if err != nil {
				return nil, fmt.Errorf("configure admin promos facade: %w", err)
			}
		}
	}
	if modules.Has(ModuleMedia) {
		store, mediaBucket, err := newMediaObjectStore(cfg)
		if err != nil {
			return nil, fmt.Errorf("configure media object store: %w", err)
		}
		mediaRepository := mediaPostgres.NewRepository(db)
		mediaCatalogReader = mediaApp.NewCatalogReader(mediaRepository, store)
		productService.WithMediaReader(mediaCatalogReader)
		mediaUploadService, err = mediaApp.NewUploadService(mediaRepository, store, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerMediaProcessor), cfg.MediaProvider, mediaBucket)
		if err != nil {
			return nil, fmt.Errorf("configure media uploads: %w", err)
		}
		mediaHandler, err := mediaApp.NewAssetUploadedHandler(mediaRepository, mediaApp.NewPureGoProcessor(store))
		if err != nil {
			return nil, fmt.Errorf("configure media processor: %w", err)
		}
		mediaOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerMediaProcessor, time.Minute, logger.Log, mediaHandler).WithTracer(observability.NewOutboxTracer())
		mediaOrphanCleanup, err = mediaApp.NewOrphanCleanupWorker(mediaRepository, store, logger.Log)
		if err != nil {
			return nil, fmt.Errorf("configure media orphan cleanup: %w", err)
		}
	}
	var videoUploadService *videoApp.DirectUploadService
	var videoPlacementFacade *videoApp.PlacementAdminFacade
	var videoWebhookService *videoApp.WebhookService
	var videoOrphanCleanup *videoApp.OrphanCleanupWorker
	var videoStorefront *videoApp.StorefrontService
	if videoEnabled {
		provider, providerErr := videoCloudflare.New(videoCloudflare.Config{
			AccountID: cfg.CloudflareStreamAccountID, APIToken: cfg.CloudflareStreamAPIToken,
			WebhookSecret:  cfg.CloudflareStreamWebhookSecret,
			AllowedOrigins: cfg.CloudflareStreamAllowedOrigins,
		})
		if providerErr != nil {
			return nil, fmt.Errorf("configure Cloudflare Stream video provider: %w", providerErr)
		}
		videoRepository := videoPostgres.NewRepository(db)
		playbackSigner, signerErr := videoCloudflare.NewPlaybackSigner(videoCloudflare.SigningConfig{
			CustomerCode:  cfg.CloudflareStreamCustomerCode,
			KeyID:         cfg.CloudflareStreamSigningKeyID,
			PrivateKeyPEM: cfg.CloudflareStreamSigningKeyPEM,
		})
		if signerErr != nil {
			return nil, fmt.Errorf("configure Cloudflare Stream playback signing: %w", signerErr)
		}
		videoStorefront, signerErr = videoApp.NewStorefrontService(videoRepository, playbackSigner, cfg.VideoPlaybackTTL)
		if signerErr != nil {
			return nil, fmt.Errorf("configure video storefront playback: %w", signerErr)
		}
		videoUploadService, providerErr = videoApp.NewDirectUploadService(videoRepository, provider, cfg.VideoProvider)
		if providerErr != nil {
			return nil, fmt.Errorf("configure video direct uploads: %w", providerErr)
		}
		videoWebhookService, providerErr = videoApp.NewWebhookService(videoRepository, provider, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher())
		if providerErr != nil {
			return nil, fmt.Errorf("configure video webhooks: %w", providerErr)
		}
		videoOrphanCleanup, providerErr = videoApp.NewOrphanCleanupWorker(videoRepository, provider, logger.Log)
		if providerErr != nil {
			return nil, fmt.Errorf("configure video orphan cleanup: %w", providerErr)
		}
		// Product video administration publishes to the Admin audit consumer,
		// which is why the video module requires admin to be enabled.
		videoPlacementFacade, providerErr = videoApp.NewPlacementAdminFacade(adminAuthorizer, videoRepository, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
		if providerErr != nil {
			return nil, fmt.Errorf("configure video placement administration: %w", providerErr)
		}
	}
	if returnsEnabled {
		policy, policyErr := returnsDomain.NewWindowEligibilityPolicy(storeConfig.ReturnWindowDays)
		if policyErr != nil {
			return nil, fmt.Errorf("configure returns policy: %w", policyErr)
		}
		returnsRepository := returnsPostgres.NewRepository(db)
		orderSnapshots := returnsOrderSnapshot.NewProvider(db)
		restockPort, restockErr := returnsInventory.NewRestockPort(db, inventoryService, storeConfig.DefaultWarehouseID)
		if restockErr != nil {
			return nil, fmt.Errorf("configure returns restock: %w", restockErr)
		}
		refundPort, refundErr := returnsFinance.NewRefundPort(db, paymentGateways)
		if refundErr != nil {
			return nil, fmt.Errorf("configure returns refund: %w", refundErr)
		}
		returnService, err = returnsApp.NewReturnService(returnsRepository, orderSnapshots, policy, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(), eventsPostgres.NewPublisher(returnsDomain.ConsumerSettlement))
		if err != nil {
			return nil, fmt.Errorf("configure returns service: %w", err)
		}
		settlementHandler, settlementErr := returnsApp.NewSettlementHandler(returnsRepository, orderSnapshots, restockPort, refundPort)
		if settlementErr != nil {
			return nil, fmt.Errorf("configure returns settlement handler: %w", settlementErr)
		}
		refundConfirmedHandler, confirmationErr := returnsApp.NewRefundConfirmedHandler(returnService)
		if confirmationErr != nil {
			return nil, fmt.Errorf("configure returns refund confirmation handler: %w", confirmationErr)
		}
		returnsOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), returnsDomain.ConsumerSettlement, time.Minute, logger.Log, settlementHandler, refundConfirmedHandler).WithTracer(observability.NewOutboxTracer())
	}

	checkoutService := checkoutApp.NewService(inventoryService, variantService, taxPolicy, checkoutDomain.Policy{
		AllowGuest:                 storeConfig.CheckoutAllowGuest,
		RequirePhone:               storeConfig.CheckoutRequirePhone,
		RequireVerifiedEmail:       storeConfig.CheckoutRequireVerifiedEmail,
		RequireVerifiedPhone:       storeConfig.CheckoutRequireVerifiedPhone,
		RequiredProfileFields:      storeConfig.CheckoutRequiredProfileFields,
		OrderNumberPrefix:          storeConfig.Code,
		SupportedDeliveryProviders: storeConfig.ShippingProviders,
		DefaultDeliveryProvider:    storeConfig.ShippingDefault,
	}, orderWorkflowService, paymentGateways.Default()).WithCarriers(deliveryCarriers).WithPriceCalculator(priceCalculator).WithCustomerVerificationReader(identityApplication.NewVerificationReader(identityPostgres.NewVerificationStatusReader(db)))
	if customerProfileService != nil {
		checkoutService.WithCustomerProfileReader(identityApplication.NewCheckoutProfileReader(customerProfileService))
	}
	var checkoutContactCapture checkoutDomain.ContactCaptureService
	if abandonedCartEnabled {
		quietHours, quietErr := abandonedApp.ParseQuietHours(cfg.AbandonedCartQuietHours)
		if quietErr != nil {
			return nil, fmt.Errorf("configure abandoned-cart quiet hours: %w", quietErr)
		}
		contactRepository := checkoutPostgres.NewContactRepository(db)
		checkoutContactCapture = checkoutApp.NewContactCaptureService(contactRepository, cartApp.NewService(newCartRepository(db, reportsEnabled, abandonedCartEnabled)), checkoutConsent.NewWriter(consentService), adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(abandonedApp.ConsumerCampaignProducer))
		campaignRepository := abandonedPostgres.NewRepository(db)
		cartReader := abandonedReaders.NewCartReader(db)
		contactProvider := abandonedReaders.NewContactProvider(db)
		producer, producerErr := abandonedApp.NewCampaignProducer(campaignRepository, cartReader, contactProvider, cfg.AbandonedCartDelays[0])
		if producerErr != nil {
			return nil, fmt.Errorf("configure abandoned-cart producer: %w", producerErr)
		}
		policy := abandonedApp.Policy{Delays: append([]time.Duration(nil), cfg.AbandonedCartDelays[:cfg.AbandonedCartMaxReminders]...), RequireMarketingConsent: cfg.AbandonedCartRequireMarketingConsent, QuietHours: quietHours}
		abandonedCartWorker = abandonedApp.NewWorker(campaignRepository, cartReader, abandonedReaders.NewConsentReader(db), notificationsPostgres.NewRepository(db), adminPostgres.NewTransactionManager(db), policy)
		abandonedCartOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), abandonedApp.ConsumerCampaignProducer, time.Minute, logger.Log, producer, abandonedApp.NewTopicConsumer(eventsDomain.TopicCheckoutEmailCaptured, producer)).WithTracer(observability.NewOutboxTracer())
	}

	categoryService := catalogApp.NewCategoryService(catalogPostgres.NewCategoryRepository(db), storeConfig.SupportedLocales).WithCache(cacheService)
	if adminEnabled {
		catalogAdminFacade, err = adminApp.NewCatalogAdminFacade(adminAuthorizer, productService, categoryService, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
		if err != nil {
			return nil, fmt.Errorf("configure admin catalog facade: %w", err)
		}
		catalogAdminFacade.WithProductEventPublisher(productEventPublisher)
		catalogAdminFacade.WithProductOptions(productOptionsService).WithInventoryAdjustment(inventoryService, storeConfig.DefaultWarehouseID)
		catalogAdminFacade.WithVariantMutations(variantService)
		if mediaCatalogReader != nil {
			catalogAdminFacade.WithMediaReader(mediaCatalogReader)
		}
		ordersAdminFacade, err = adminApp.NewOrdersAdminFacade(adminAuthorizer, orderWorkflowService, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
		if err != nil {
			return nil, fmt.Errorf("configure admin orders facade: %w", err)
		}
		if operationalWorkflowPolicy != nil {
			ordersAdminFacade.WithStatusWorkflow(operationalWorkflowPolicy, orderWorkflowService)
			ordersAdminFacade.WithWorkflowConfiguration(operationalWorkflowPolicy)
		}
		// Badges, reviews and SEO were left unexposed until each mutation could
		// carry an audit event in its own transaction. The facade supplies that,
		// and attaches only the modules this deployment enabled.
		contentAdminFacade, err = adminApp.NewContentAdminFacade(adminAuthorizer, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
		if err != nil {
			return nil, fmt.Errorf("configure admin content facade: %w", err)
		}
		contentAdminFacade.WithBadges(enabledBadges).WithReviews(enabledReviews).WithSEO(enabledSEO)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain SQL database: %w", err)
	}
	readinessChecks := []management.ReadinessCheck{{
		Name: "postgres",
		Check: func(ctx context.Context) error {
			return sqlDB.PingContext(ctx)
		},
	}}
	if platform.Redis != nil {
		readinessChecks = append(readinessChecks, management.ReadinessCheck{
			Name: "redis",
			Check: func(ctx context.Context) error {
				return platform.Redis.Raw().Ping(ctx).Err()
			},
		})
	}
	deliveryTracker := deliveryApp.NewTracker(deliveryPostgres.NewTrackingStore(db), deliveryCarriers)
	if deliveryOrderTransitioner != nil {
		deliveryTracker = deliveryApp.NewTracker(deliveryPostgres.NewTrackingStore(db), deliveryCarriers, deliveryOrderTransitioner)
	}
	application := &Application{
		Config: cfg, StoreConfig: storeConfig, TokenMaker: tokenMaker,
		CatalogCategoryService:    categoryService,
		CatalogProductService:     productService,
		CatalogVariantService:     variantService,
		CartService:               cartApp.NewService(newCartRepository(db, reportsEnabled, abandonedCartEnabled)),
		CheckoutService:           checkoutService,
		CheckoutContactCapture:    checkoutContactCapture,
		CheckoutRecovery:          checkoutApp.NewRecoveryService(orderWorkflowService, paymentGateways, logger.Log),
		CheckoutExpiry:            checkoutApp.NewExpiryService(orderWorkflowService),
		OrderWorkflowService:      orderWorkflowService,
		InventoryService:          inventoryService,
		InventoryAvailability:     inventoryRepository,
		InventoryCleanup:          inventoryApp.NewCleanup(inventoryRepository),
		OrderService:              ordersApp.NewService(ordersPostgres.NewRepository(db)),
		IdentityAuthService:       identityAuthService,
		IdentityProfileService:    identityProfileService,
		CustomerProfileService:    customerProfileService,
		WishlistService:           enabledWishlist,
		ComparisonService:         enabledComparison,
		ReviewsService:            enabledReviews,
		SEOService:                enabledSEO,
		BadgesService:             enabledBadges,
		PaymentGateways:           paymentGateways,
		PaymentWebhookService:     paymentWebhookService,
		DeliveryCarriers:          deliveryCarriers,
		DeliveryLocations:         deliveryApp.NewLocationService(deliveryCarriers, cacheService),
		DeliveryDispatcher:        deliveryApp.NewDispatcher(deliveryPostgres.NewJobStore(db), deliveryCarriers, time.Minute),
		DeliveryTracker:           deliveryTracker,
		OutboxWorker:              eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerNotifications, time.Minute, logger.Log, outboxHandlers...).WithTracer(observability.NewOutboxTracer()),
		AdminAuditOutboxWorker:    adminAuditOutboxWorker,
		SearchOutboxWorker:        searchOutboxWorker,
		MediaOutboxWorker:         mediaOutboxWorker,
		ReportsOutboxWorker:       reportsOutboxWorker,
		OutboxRetention:           outboxRetention,
		SyncDispatcher:            syncDispatcher,
		MediaOrphanCleanup:        mediaOrphanCleanup,
		SearchService:             searchService,
		MediaUploadService:        mediaUploadService,
		VideoUploadService:        videoUploadService,
		VideoPlacementFacade:      videoPlacementFacade,
		VideoWebhookService:       videoWebhookService,
		VideoOrphanCleanup:        videoOrphanCleanup,
		VideoStorefront:           videoStorefront,
		ReportsQueryService:       reportsQueryService,
		ReportsRebuilder:          reportsRebuilder,
		ReturnService:             returnService,
		ReturnsOutboxWorker:       returnsOutboxWorker,
		NotificationWorker:        notificationWorker,
		AvailabilityService:       availabilityService,
		AvailabilityOutboxWorker:  availabilityOutboxWorker,
		AbandonedCartWorker:       abandonedCartWorker,
		AbandonedCartOutboxWorker: abandonedCartOutboxWorker,
		SupportService:            supportService,
		ConsentService:            consentService,
		AdminAuthorizer:           adminAuthorizer,
		PromosAdminFacade:         promosAdminFacade,
		CatalogAdminFacade:        catalogAdminFacade,
		ContentAdminFacade:        contentAdminFacade,
		OrdersAdminFacade:         ordersAdminFacade,
		TaxPolicy:                 taxPolicy,
		HTTP:                      httpDependencies,
		Management:                management.NewServer(cfg.ManagementAddr, readinessChecks...),
		resourceCloser:            platform.closer(),
		telemetryShutdown:         platform.telemetryShutdown,
	}
	platform.commit()
	return application, nil
}

type closeAll []io.Closer

func (closers closeAll) Close() error {
	var first error
	for index := len(closers) - 1; index >= 0; index-- {
		if closer := closers[index]; closer != nil {
			if err := closer.Close(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func inventoryMode(value string) inventoryDomain.Mode {
	if value == "internal" {
		return inventoryDomain.ModeInternal
	}
	return inventoryDomain.ModeExternal
}

// orderWorkflowEventRoutes is the explicit subscription policy for events
// emitted by the cross-context Order Workflow. Keep it topic-specific: workers
// must never claim topics for which they have no handler.
func orderWorkflowEventRoutes(notificationsEnabled, reportsEnabled, returnsEnabled bool) map[string][]string {
	routes := make(map[string][]string, 3)
	if notificationsEnabled {
		routes[eventsDomain.TopicOrderPaid] = append(routes[eventsDomain.TopicOrderPaid], eventsDomain.ConsumerNotifications)
	}
	if reportsEnabled {
		routes[eventsDomain.TopicCheckoutStarted] = append(routes[eventsDomain.TopicCheckoutStarted], eventsDomain.ConsumerReportsProjection)
		routes[eventsDomain.TopicOrderPaid] = append(routes[eventsDomain.TopicOrderPaid], eventsDomain.ConsumerReportsProjection)
		routes[eventsDomain.TopicOrderRefunded] = append(routes[eventsDomain.TopicOrderRefunded], eventsDomain.ConsumerReportsProjection)
	}
	if returnsEnabled {
		// This closes an RMA only after the trusted payment workflow has
		// durably confirmed the gateway refund.
		routes[eventsDomain.TopicOrderRefunded] = append(routes[eventsDomain.TopicOrderRefunded], returnsDomain.ConsumerSettlement)
	}
	return routes
}

func newCartRepository(db *gorm.DB, reportsEnabled, abandonedCartEnabled bool) *cartPostgres.Repository {
	repository := cartPostgres.NewRepository(db)
	consumers := make([]string, 0, 2)
	if reportsEnabled {
		consumers = append(consumers, eventsDomain.ConsumerReportsProjection)
	}
	if abandonedCartEnabled {
		consumers = append(consumers, abandonedApp.ConsumerCampaignProducer)
	}
	if len(consumers) > 0 {
		repository.WithEventPublisher(eventsPostgres.NewPublisher(consumers...))
	}
	return repository
}

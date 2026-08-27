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

	googleAuthAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/auth/google"
	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	badgesPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/repository/postgres"
	badgesService "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/service"
	cartApp "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/application"
	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	cartPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/repository/postgres"
	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	catalogPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/repository/postgres"
	checkoutApp "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/application"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	comparisonDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
	comparisonPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/repository/postgres"
	comparisonService "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/service"
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
	platformCache "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/cache"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/management"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	workflowPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/orderworkflow"
	platformRedis "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/redis"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	promosApp "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
	promosPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/repository/postgres"
	reportsOrderAnalytics "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/adapter/orderanalytics"
	reportsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application"
	reportsProjectors "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application/projectors"
	reportsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
	reportsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/repository/postgres"
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
	sharedCache "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
	wishlistDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	wishlistPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/repository/postgres"
	wishlistService "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/service"
)

// Application exposes only services that belong to the active clean-slate graph.
type Application struct {
	Config                 *config.Config
	StoreConfig            StoreConfig
	TokenMaker             token.Maker
	CatalogCategoryService catalogDomain.CategoryService
	CatalogProductService  catalogDomain.ProductService
	CatalogVariantService  catalogDomain.VariantService
	CartService            cartDomain.Service
	CheckoutService        checkoutDomain.Service
	CheckoutRecovery       *checkoutApp.RecoveryService
	CheckoutExpiry         *checkoutApp.ExpiryService
	OrderWorkflowService   orderWorkflowDomain.Service
	InventoryService       inventoryDomain.Service
	InventoryCleanup       *inventoryApp.Cleanup
	OrderService           ordersDomain.Service
	IdentityAuthService    identityDomain.AuthService
	IdentityProfileService identityDomain.ProfileService
	WishlistService        wishlistDomain.Service
	ComparisonService      comparisonDomain.Service
	ReviewsService         reviewsDomain.Service
	SEOService             seoDomain.Service
	BadgesService          badgesDomain.Service
	PaymentGateways        *paymentsApp.Registry
	PaymentWebhookService  *paymentsApp.WebhookService
	DeliveryCarriers       *deliveryApp.Registry
	DeliveryLocations      *deliveryApp.LocationService
	DeliveryDispatcher     *deliveryApp.Dispatcher
	DeliveryTracker        *deliveryApp.Tracker
	OutboxWorker           *eventsApp.OutboxWorker
	AdminAuditOutboxWorker *eventsApp.OutboxWorker
	SearchOutboxWorker     *eventsApp.OutboxWorker
	MediaOutboxWorker      *eventsApp.OutboxWorker
	ReportsOutboxWorker    *eventsApp.OutboxWorker
	OutboxRetention        *eventsApp.RetentionWorker
	MediaOrphanCleanup     *mediaApp.OrphanCleanupWorker
	SearchService          searchDomain.SearchService
	MediaUploadService     *mediaApp.UploadService
	ReportsQueryService    reportsDomain.QueryService
	ReportsRebuilder       *reportsApp.ReportsRebuilder
	AdminAuthorizer        adminDomain.Authorizer
	PromosAdminFacade      *adminApp.PromosAdminFacade
	CatalogAdminFacade     *adminApp.CatalogAdminFacade
	OrdersAdminFacade      *adminApp.OrdersAdminFacade
	TaxPolicy              tax.Calculator
	HTTP                   HTTPDependencies
	Management             *management.Server
	resourceCloser         io.Closer
	telemetryShutdown      func(context.Context) error
	workerMu               sync.Mutex
	workerCancel           context.CancelFunc
	workerWG               sync.WaitGroup
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

	// Redis is opt-in. When disabled, cache misses and local, per-process login
	// limiting preserve a fully functional constrained-environment deployment.
	telemetryShutdown, err := observability.Init(context.Background(), observability.Config{
		Enabled:     cfg.OTelEnabled,
		Endpoint:    cfg.OTelEndpoint,
		ServiceName: "ecommerce-core",
		Environment: cfg.Env,
	})
	if err != nil {
		return nil, fmt.Errorf("configure observability: %w", err)
	}

	cacheService := sharedCache.Service(sharedCache.NewNoOpService())
	loginLimiter := ratelimit.Service(ratelimit.NewLocalService())
	var resourceClosers []io.Closer
	var redisClient *platformRedis.Client
	bootstrapComplete := false
	defer func() {
		if !bootstrapComplete {
			for _, closer := range resourceClosers {
				_ = closer.Close()
			}
		}
		if !bootstrapComplete && telemetryShutdown != nil {
			_ = telemetryShutdown(context.Background())
		}
	}()
	if cfg.RedisEnabled {
		var redisErr error
		redisClient, redisErr = platformRedis.Connect(context.Background(), cfg.RedisURL)
		if redisErr != nil {
			return nil, fmt.Errorf("configure Redis: %w", redisErr)
		}
		resourceClosers = append(resourceClosers, redisClient)
		cacheService = platformCache.NewRedisAdapter(redisClient.Raw(), storeConfig.Code+":")
		loginLimiter = platformRedis.NewFixedWindowLimiter(redisClient.Raw())
	}
	adminEnabled := contains(storeConfig.EnabledModules, "admin")
	var adminAuthorizer adminDomain.Authorizer
	if adminEnabled {
		adminAuthorizer, err = adminApp.NewAuthorizer(adminPostgres.NewRepository(db), cacheService, 0)
		if err != nil {
			return nil, fmt.Errorf("configure admin RBAC: %w", err)
		}
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

	variantService := catalogApp.NewVariantService(catalogPostgres.NewVariantRepository(db), storeConfig.SupportedLocales, storeConfig.Currency)
	inventoryRepository := inventoryPostgres.NewRepository(db)
	inventoryService := inventoryApp.NewService(inventoryMode(storeConfig.InventoryMode), inventoryRepository)
	notificationsEnabled := contains(storeConfig.EnabledModules, "notifications")
	reportsEnabled := contains(storeConfig.EnabledModules, "reports")
	eventConsumers := make([]string, 0, 1)
	if notificationsEnabled {
		eventConsumers = append(eventConsumers, eventsDomain.ConsumerNotifications)
	}
	if reportsEnabled {
		eventConsumers = append(eventConsumers, eventsDomain.ConsumerReportsProjection)
	}
	// The workflow repository is PostgreSQL infrastructure. It is deliberately
	// outside core so core/application code does not depend on an Orders or
	// Inventory repository implementation.
	workflowRepository := workflowPostgres.NewRepository(db, contains(storeConfig.EnabledModules, "sync")).
		WithEventPublisher(eventsPostgres.NewPublisher(eventConsumers...))
	basePriceCalculator, err := checkoutDomain.NewCheckoutPriceCalculator(taxPolicy)
	if err != nil {
		return nil, err
	}
	var priceCalculator checkoutDomain.PriceCalculator = basePriceCalculator
	var promosRepository *promosPostgres.Repository
	if contains(storeConfig.EnabledModules, "promos") {
		promosRepository = promosPostgres.NewRepository(db)
		workflowRepository.WithTransactionHook(promosApp.NewWorkflowHook(promosRepository))
		priceCalculator = promosApp.NewPromoCalculatorDecorator(priceCalculator, promosRepository)
	}
	orderWorkflowService := orderWorkflowApp.NewService(workflowRepository)
	paymentWebhookService := paymentsApp.NewWebhookService(paymentGateways, paymentsPostgres.NewWebhookEventStore(db), orderWorkflowService)
	var enabledWishlist wishlistDomain.Service
	if contains(storeConfig.EnabledModules, "wishlist") {
		enabledWishlist = wishlistService.New(wishlistPostgres.NewRepository(db))
	}
	var enabledComparison comparisonDomain.Service
	if contains(storeConfig.EnabledModules, "comparison") {
		enabledComparison = comparisonService.New(comparisonPostgres.NewRepository(db), storeConfig.ComparisonMaxItems)
	}
	productService := catalogApp.NewProductService(catalogPostgres.NewProductRepository(db), storeConfig.SupportedLocales)
	searchEnabled := contains(storeConfig.EnabledModules, "search")
	var enabledSEO seoDomain.Service
	if contains(storeConfig.EnabledModules, "seo") {
		repository := seoPostgres.NewRepository(db)
		enabledSEO = seoService.New(repository)
		productService.WithSEOReader(repository)
	}
	var enabledBadges badgesDomain.Service
	if contains(storeConfig.EnabledModules, "badges") {
		repository := badgesPostgres.NewRepository(db)
		enabledBadges = badgesService.New(repository)
		productService.WithBadgeReader(repository)
	}
	var enabledReviews reviewsDomain.Service
	if contains(storeConfig.EnabledModules, "reviews") {
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
	if contains(storeConfig.EnabledModules, "user_profiles") {
		identityProfileService = identityService.NewProfileService(*storeConfig.ProfilePolicy, identityPostgres.NewProfileRepository(db))
	}
	outboxHandlers := make([]eventsApp.Consumer, 0, 1)
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
	}
	var promosAdminFacade *adminApp.PromosAdminFacade
	var catalogAdminFacade *adminApp.CatalogAdminFacade
	var ordersAdminFacade *adminApp.OrdersAdminFacade
	var adminAuditOutboxWorker *eventsApp.OutboxWorker
	var searchOutboxWorker *eventsApp.OutboxWorker
	var mediaOutboxWorker *eventsApp.OutboxWorker
	var reportsOutboxWorker *eventsApp.OutboxWorker
	outboxRetention, err := eventsApp.NewRetentionWorker(eventsPostgres.NewRetentionStore(db), cfg.OutboxDoneRetention, 1000, logger.Log)
	if err != nil {
		return nil, fmt.Errorf("configure outbox retention: %w", err)
	}
	var mediaOrphanCleanup *mediaApp.OrphanCleanupWorker
	var productEventPublisher eventsDomain.TransactionalEventPublisher
	var searchService searchDomain.SearchService
	var mediaUploadService *mediaApp.UploadService
	var mediaCatalogReader *mediaApp.CatalogReader
	var reportsQueryService reportsDomain.QueryService
	var reportsRebuilder *reportsApp.ReportsRebuilder
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
		reportsOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerReportsProjection, time.Minute, logger.Log, handler, refundHandler, cartFunnel, checkoutFunnel)
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
		resourceClosers = append(resourceClosers, index)
		handler, err := searchApp.NewProductChangedHandler(searchCatalog.NewSnapshotProvider(productService), index)
		if err != nil {
			return nil, fmt.Errorf("configure search indexer: %w", err)
		}
		searchService, err = searchApp.NewSearchService(index)
		if err != nil {
			return nil, fmt.Errorf("configure search service: %w", err)
		}
		searchOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerSearchIndexer, time.Minute, logger.Log, handler)
		productEventPublisher = eventsPostgres.NewPublisher(eventsDomain.ConsumerSearchIndexer)
	}
	if adminEnabled {
		auditHandler := adminApp.NewAdminAuditEventHandler(adminPostgres.NewAuditRepository(db))
		adminAuditOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerAdminAudit, time.Minute, logger.Log, auditHandler)
		if promosRepository != nil {
			promosAdminFacade, err = adminApp.NewPromosAdminFacade(adminAuthorizer, promosApp.NewAdminService(promosRepository), adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
			if err != nil {
				return nil, fmt.Errorf("configure admin promos facade: %w", err)
			}
		}
	}
	if contains(storeConfig.EnabledModules, "media") {
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
		mediaOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerMediaProcessor, time.Minute, logger.Log, mediaHandler)
		mediaOrphanCleanup, err = mediaApp.NewOrphanCleanupWorker(mediaRepository, store, logger.Log)
		if err != nil {
			return nil, fmt.Errorf("configure media orphan cleanup: %w", err)
		}
	}

	checkoutService := checkoutApp.NewService(inventoryService, variantService, taxPolicy, checkoutDomain.Policy{
		AllowGuest:                 storeConfig.CheckoutAllowGuest,
		RequirePhone:               storeConfig.CheckoutRequirePhone,
		OrderNumberPrefix:          storeConfig.Code,
		SupportedDeliveryProviders: storeConfig.ShippingProviders,
		DefaultDeliveryProvider:    storeConfig.ShippingDefault,
	}, orderWorkflowService, paymentGateways.Default()).WithCarriers(deliveryCarriers).WithPriceCalculator(priceCalculator)

	categoryService := catalogApp.NewCategoryService(catalogPostgres.NewCategoryRepository(db), storeConfig.SupportedLocales).WithCache(cacheService)
	if adminEnabled {
		catalogAdminFacade, err = adminApp.NewCatalogAdminFacade(adminAuthorizer, productService, categoryService, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
		if err != nil {
			return nil, fmt.Errorf("configure admin catalog facade: %w", err)
		}
		catalogAdminFacade.WithProductEventPublisher(productEventPublisher)
		if mediaCatalogReader != nil {
			catalogAdminFacade.WithMediaReader(mediaCatalogReader)
		}
		ordersAdminFacade, err = adminApp.NewOrdersAdminFacade(adminAuthorizer, orderWorkflowService, adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
		if err != nil {
			return nil, fmt.Errorf("configure admin orders facade: %w", err)
		}
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
	if redisClient != nil {
		readinessChecks = append(readinessChecks, management.ReadinessCheck{
			Name: "redis",
			Check: func(ctx context.Context) error {
				return redisClient.Raw().Ping(ctx).Err()
			},
		})
	}
	application := &Application{
		Config: cfg, StoreConfig: storeConfig, TokenMaker: tokenMaker,
		CatalogCategoryService: categoryService,
		CatalogProductService:  productService,
		CatalogVariantService:  variantService,
		CartService:            cartApp.NewService(newCartRepository(db, reportsEnabled)),
		CheckoutService:        checkoutService,
		CheckoutRecovery:       checkoutApp.NewRecoveryService(orderWorkflowService, paymentGateways, logger.Log),
		CheckoutExpiry:         checkoutApp.NewExpiryService(orderWorkflowService),
		OrderWorkflowService:   orderWorkflowService,
		InventoryService:       inventoryService,
		InventoryCleanup:       inventoryApp.NewCleanup(inventoryRepository),
		OrderService:           ordersApp.NewService(ordersPostgres.NewRepository(db)),
		IdentityAuthService:    identityAuthService,
		IdentityProfileService: identityProfileService,
		WishlistService:        enabledWishlist,
		ComparisonService:      enabledComparison,
		ReviewsService:         enabledReviews,
		SEOService:             enabledSEO,
		BadgesService:          enabledBadges,
		PaymentGateways:        paymentGateways,
		PaymentWebhookService:  paymentWebhookService,
		DeliveryCarriers:       deliveryCarriers,
		DeliveryLocations:      deliveryApp.NewLocationService(deliveryCarriers),
		DeliveryDispatcher:     deliveryApp.NewDispatcher(deliveryPostgres.NewJobStore(db), deliveryCarriers, time.Minute),
		DeliveryTracker:        deliveryApp.NewTracker(deliveryPostgres.NewTrackingStore(db), deliveryCarriers),
		OutboxWorker:           eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerNotifications, time.Minute, logger.Log, outboxHandlers...),
		AdminAuditOutboxWorker: adminAuditOutboxWorker,
		SearchOutboxWorker:     searchOutboxWorker,
		MediaOutboxWorker:      mediaOutboxWorker,
		ReportsOutboxWorker:    reportsOutboxWorker,
		OutboxRetention:        outboxRetention,
		MediaOrphanCleanup:     mediaOrphanCleanup,
		SearchService:          searchService,
		MediaUploadService:     mediaUploadService,
		ReportsQueryService:    reportsQueryService,
		ReportsRebuilder:       reportsRebuilder,
		AdminAuthorizer:        adminAuthorizer,
		PromosAdminFacade:      promosAdminFacade,
		CatalogAdminFacade:     catalogAdminFacade,
		OrdersAdminFacade:      ordersAdminFacade,
		TaxPolicy:              taxPolicy,
		HTTP:                   httpDependencies,
		Management:             management.NewServer(cfg.ManagementAddr, readinessChecks...),
		resourceCloser:         closeAll(resourceClosers),
		telemetryShutdown:      telemetryShutdown,
	}
	bootstrapComplete = true
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

func newCartRepository(db *gorm.DB, reportsEnabled bool) *cartPostgres.Repository {
	repository := cartPostgres.NewRepository(db)
	if reportsEnabled {
		repository.WithEventPublisher(eventsPostgres.NewPublisher(eventsDomain.ConsumerReportsProjection))
	}
	return repository
}

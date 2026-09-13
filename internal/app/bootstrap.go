// Package app contains the clean-slate Composition Root.
package app

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	abandonedApp "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/application"
	syncHTTPExport "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/sync/httpexport"
	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	availabilityApp "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/application"
	availabilityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	badgesPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/repository/postgres"
	badgesService "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/service"
	cartApp "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/application"
	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	cartPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/repository/postgres"
	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	catalogPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/repository/postgres"
	checkoutIdentity "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/adapter/identity"
	checkoutApp "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/application"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	comparisonDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
	comparisonPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/repository/postgres"
	comparisonService "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/service"
	consentApp "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/application"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events/application"
	localeApp "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/application"
	localePostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/core/locale/repository/postgres"
	orderWorkflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/tax"
	deliveryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/application"
	deliveryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	identityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/repository/postgres"
	inventoryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/application"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	inventoryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/repository/postgres"
	mediaApp "github.com/VladHrytsaiuk/ecommerce-core/internal/media/application"
	notificationsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/application"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	ordersApp "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/application"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	ordersPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/repository/postgres"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/management"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/observability"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	promosApp "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
	reportsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/application"
	reportsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/domain"
	returnsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/application"
	returnsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	reviewsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/repository/postgres"
	reviewsService "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/service"
	searchDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
	seoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/repository/postgres"
	seoService "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/service"
	supportApp "github.com/VladHrytsaiuk/ecommerce-core/internal/support/application"
	syncApp "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/application"
	syncPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/sync/repository/postgres"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
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
	NotificationRetention     *notificationsApp.RetentionWorker
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

	httpDependencies, err := buildHTTPDependencies(cfg, storeConfig, tokenMaker, loginLimiter)
	if err != nil {
		return nil, err
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
		availability := buildAvailability(storeConfig, db)
		availabilityService, availabilityOutboxWorker = availability.Service, availability.Worker
		// Inventory publishes the stock transition that wakes the consumer, so
		// the publisher is attached where that transition happens.
		inventoryService.WithAvailabilityPublisher(eventsPostgres.NewPublisher(availabilityDomain.ConsumerAvailabilityNotifications))
	}
	var supportService *supportApp.Service
	if supportEnabled {
		supportService = buildSupport(storeConfig, db, loginLimiter)
	}
	var consentService *consentApp.Service
	if consentEnabled {
		consentService, err = buildConsent(cfg, db)
		if err != nil {
			return nil, err
		}
	}
	// The workflow repository is PostgreSQL infrastructure. It is deliberately
	// outside core so core/application code does not depend on an Orders or
	// Inventory repository implementation.
	commerce, commerceErr := buildCommerce(db, modules, taxPolicy, paymentGateways, workflowEventRoutes)
	if commerceErr != nil {
		return nil, commerceErr
	}
	orderWorkflowService := commerce.Workflow
	operationalWorkflowPolicy := commerce.Policy
	priceCalculator := commerce.PriceCalculator
	promosRepository := commerce.Promos
	paymentWebhookService := commerce.PaymentWebhooks
	deliveryOrderTransitioner := commerce.DeliveryBridge
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
	identity, identityErr := buildIdentity(cfg, storeConfig, db, tokenMaker, modules,
		newUserLoginObserver(newWishlistLoginObserver(enabledWishlist), newComparisonLoginObserver(enabledComparison)))
	if identityErr != nil {
		return nil, identityErr
	}
	identityAuthService := identity.Auth
	identityProfileService, customerProfileService := identity.Profiles, identity.CustomerProfile
	outboxHandlers := make([]eventsApp.Consumer, 0, 1)
	var notificationWorker *notificationsApp.DurableWorker
	// Built only when the module is enabled. A worker with no handlers is not
	// harmless: it claims this consumer's deliveries and, until the handler
	// check above existed, acknowledged them. A deployment that turned
	// notifications off with a non-empty queue erased its pending order
	// confirmations. It is also a database round trip every five seconds for a
	// module that is not present.
	// notification_jobs holds one row per message ever sent, with the
	// recipient's address and, on the scheduled path, the rendered body. It had
	// no window at all. The worker runs only where the module does, because
	// without it nothing writes to the table.
	var notificationRetention *notificationsApp.RetentionWorker
	var notificationsOutboxWorker *eventsApp.OutboxWorker
	if notificationsEnabled {
		retention, retentionErr := notificationsApp.NewRetentionWorker(
			notificationsPostgres.NewRepository(db),
			cfg.NotificationSentRetention, cfg.NotificationDeadRetention, 1000, logger.Log)
		if retentionErr != nil {
			return nil, fmt.Errorf("configure notification retention: %w", retentionErr)
		}
		notificationRetention = retention.WithMetrics(observability.NewNotificationMetrics())
		notifications, notificationsErr := buildNotifications(cfg, storeConfig, db)
		if notificationsErr != nil {
			return nil, notificationsErr
		}
		notificationWorker = notifications.Worker
		outboxHandlers = append(outboxHandlers, notifications.Handlers...)
		notificationsOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerNotifications, time.Minute, logger.Log, outboxHandlers...).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics())
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
		syncDispatcher = syncApp.NewDispatcher(syncPostgres.NewOutboxStore(db), exporter, cfg.SyncRetryDelay, cfg.SyncDispatchLease, cfg.SyncMaxAttempts).WithLogger(logger.Log)
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
		reports, reportsErr := buildReports(cfg, db)
		if reportsErr != nil {
			return nil, reportsErr
		}
		reportsQueryService, reportsRebuilder, reportsOutboxWorker = reports.Queries, reports.Rebuilder, reports.Worker
	}
	if searchEnabled {
		search, searchErr := buildSearch(cfg, storeConfig, db, productService)
		// The index client is registered even on failure: it may already be
		// open, and rollback must close it rather than leak the connection.
		platform.adopt(search.Index)
		if searchErr != nil {
			return nil, searchErr
		}
		searchService, searchOutboxWorker = search.Service, search.Worker
		productEventPublisher = search.ProductPublisher
	}
	if adminEnabled {
		auditHandler := adminApp.NewAdminAuditEventHandler(adminPostgres.NewAuditRepository(db))
		adminAuditOutboxWorker = eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerAdminAudit, time.Minute, logger.Log, auditHandler).WithTracer(observability.NewOutboxTracer()).WithMetrics(observability.NewOutboxMetrics())
		if promosRepository != nil {
			promosAdminFacade, err = adminApp.NewPromosAdminFacade(adminAuthorizer, promosApp.NewAdminService(promosRepository), adminPostgres.NewTransactionManager(db), eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))
			if err != nil {
				return nil, fmt.Errorf("configure admin promos facade: %w", err)
			}
		}
	}
	if modules.Has(ModuleMedia) {
		media, mediaErr := buildMedia(cfg, db)
		if mediaErr != nil {
			return nil, mediaErr
		}
		mediaUploadService, mediaCatalogReader = media.Uploads, media.CatalogReader
		mediaOutboxWorker, mediaOrphanCleanup = media.Worker, media.OrphanCleanup
		productService.WithMediaReader(mediaCatalogReader)
	}
	var videoUploadService *videoApp.DirectUploadService
	var videoPlacementFacade *videoApp.PlacementAdminFacade
	var videoWebhookService *videoApp.WebhookService
	var videoOrphanCleanup *videoApp.OrphanCleanupWorker
	var videoStorefront *videoApp.StorefrontService
	if videoEnabled {
		video, videoErr := buildVideo(cfg, db, adminAuthorizer)
		if videoErr != nil {
			return nil, videoErr
		}
		videoUploadService, videoPlacementFacade = video.Uploads, video.PlacementFacade
		videoWebhookService, videoOrphanCleanup = video.Webhooks, video.OrphanCleanup
		videoStorefront = video.Storefront
	}
	if returnsEnabled {
		returns, returnsErr := buildReturns(storeConfig, db, inventoryService, paymentGateways, notificationsEnabled)
		if returnsErr != nil {
			return nil, returnsErr
		}
		returnService, returnsOutboxWorker = returns.Service, returns.Worker
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
	}, orderWorkflowService, paymentGateways.Default()).WithCarriers(deliveryCarriers).WithPriceCalculator(priceCalculator).WithCustomerVerificationReader(checkoutIdentity.NewVerificationReader(identityPostgres.NewVerificationStatusReader(db)))
	if customerProfileService != nil {
		checkoutService.WithCustomerProfileReader(checkoutIdentity.NewCheckoutProfileReader(customerProfileService))
	}
	var checkoutContactCapture checkoutDomain.ContactCaptureService
	if abandonedCartEnabled {
		abandoned, abandonedErr := buildAbandonedCart(cfg, storeConfig, db, consentService, func() *cartApp.Service {
			return cartApp.NewService(newCartRepository(db, reportsEnabled, abandonedCartEnabled))
		})
		if abandonedErr != nil {
			return nil, abandonedErr
		}
		abandonedCartWorker, abandonedCartOutboxWorker = abandoned.Worker, abandoned.OutboxWorker
		checkoutContactCapture = abandoned.ContactCapture
	}

	categoryService := catalogApp.NewCategoryService(catalogPostgres.NewCategoryRepository(db), storeConfig.SupportedLocales).WithCache(cacheService)
	if adminEnabled {
		facades, facadeErr := buildAdminFacades(db, adminAuthorizer, storeConfig, adminFacadeDeps{
			Products: productService, Categories: categoryService, Variants: variantService,
			ProductOptions: productOptionsService, Inventory: inventoryService,
			MediaReader: mediaCatalogReader, ProductPublisher: productEventPublisher,
			Workflow: orderWorkflowService, WorkflowPolicy: operationalWorkflowPolicy,
			Badges: enabledBadges, Reviews: enabledReviews, SEO: enabledSEO,
		})
		if facadeErr != nil {
			return nil, facadeErr
		}
		catalogAdminFacade, ordersAdminFacade, contentAdminFacade = facades.Catalog, facades.Orders, facades.Content
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
	deliveryTracker := deliveryApp.NewTracker(deliveryPostgres.NewTrackingStore(db), deliveryCarriers).WithLogger(logger.Log)
	if deliveryOrderTransitioner != nil {
		deliveryTracker = deliveryApp.NewTracker(deliveryPostgres.NewTrackingStore(db), deliveryCarriers, deliveryOrderTransitioner).WithLogger(logger.Log)
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
		CheckoutExpiry:            checkoutApp.NewExpiryService(orderWorkflowService).WithLogger(logger.Log),
		OrderWorkflowService:      orderWorkflowService,
		InventoryService:          inventoryService,
		InventoryAvailability:     inventoryRepository,
		InventoryCleanup:          inventoryApp.NewCleanup(inventoryRepository).WithLogger(logger.Log),
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
		DeliveryDispatcher:        deliveryApp.NewDispatcher(deliveryPostgres.NewJobStore(db), deliveryCarriers, time.Minute).WithLogger(logger.Log),
		DeliveryTracker:           deliveryTracker,
		OutboxWorker:              notificationsOutboxWorker,
		AdminAuditOutboxWorker:    adminAuditOutboxWorker,
		SearchOutboxWorker:        searchOutboxWorker,
		MediaOutboxWorker:         mediaOutboxWorker,
		ReportsOutboxWorker:       reportsOutboxWorker,
		OutboxRetention:           outboxRetention,
		NotificationRetention:     notificationRetention,
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

// newCartRepository routes the cart's two topics per topic rather than fanning
// both out to every interested consumer.
//
// A plain NewPublisher is blind to the topic: it writes a delivery row for each
// listed consumer on every event the repository publishes. That gave the
// reports projection a cart.updated delivery it has no handler for, and the
// campaign producer a carts.created one — both of which the outbox worker then
// acknowledged without doing anything. Only reports cares about a cart being
// created; only the campaign producer cares about it changing.
func newCartRepository(db *gorm.DB, reportsEnabled, abandonedCartEnabled bool) *cartPostgres.Repository {
	repository := cartPostgres.NewRepository(db)
	if routes := cartEventRoutes(reportsEnabled, abandonedCartEnabled); len(routes) > 0 {
		// An unrouted topic still records its event; it simply produces no
		// delivery, which is the correct outcome for an event nobody consumes.
		repository.WithEventPublisher(eventsPostgres.NewTopicPublisher(routes))
	}
	return repository
}

// cartEventRoutes is the subscription policy for the cart's two topics, kept
// beside orderWorkflowEventRoutes and for the same reason: a consumer must
// only be sent a topic it has a handler for.
//
// Reports counts carts as they are created; the recovery campaign reacts to
// them changing. Neither has a handler for the other's topic.
func cartEventRoutes(reportsEnabled, abandonedCartEnabled bool) map[string][]string {
	routes := make(map[string][]string, 2)
	if reportsEnabled {
		routes[eventsDomain.TopicCartCreated] = append(routes[eventsDomain.TopicCartCreated], eventsDomain.ConsumerReportsProjection)
	}
	if abandonedCartEnabled {
		routes[eventsDomain.TopicCartUpdated] = append(routes[eventsDomain.TopicCartUpdated], abandonedApp.ConsumerCampaignProducer)
	}
	return routes
}

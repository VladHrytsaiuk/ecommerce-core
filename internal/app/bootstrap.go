// Package app contains the clean-slate Composition Root.
package app

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	googleAuthAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/auth/google"
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
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	identityPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/repository/postgres"
	identityService "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/service"
	inventoryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/application"
	inventoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/domain"
	inventoryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/inventory/repository/postgres"
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
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	workflowPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/orderworkflow"
	platformRedis "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/redis"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	promosApp "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/application"
	promosPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/repository/postgres"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	reviewsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/repository/postgres"
	reviewsService "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/service"
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
	DeliveryDispatcher     *deliveryApp.Dispatcher
	DeliveryTracker        *deliveryApp.Tracker
	OutboxWorker           *eventsApp.OutboxWorker
	TaxPolicy              tax.Calculator
	HTTP                   HTTPDependencies
	resourceCloser         io.Closer
	workerMu               sync.Mutex
	workerCancel           context.CancelFunc
	workerWG               sync.WaitGroup
}

type HTTPDependencies struct {
	Recovery         gin.HandlerFunc
	Timeout          gin.HandlerFunc
	RequestLogging   gin.HandlerFunc
	CORS             gin.HandlerFunc
	Swagger          gin.HandlerFunc
	Health           gin.HandlerFunc
	LocaleMiddleware gin.HandlerFunc
	OptionalAuth     gin.HandlerFunc
	LoginRateLimit   gin.HandlerFunc
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
	cacheService := sharedCache.Service(sharedCache.NewNoOpService())
	loginLimiter := ratelimit.Service(ratelimit.NewLocalService())
	var resourceCloser io.Closer
	bootstrapComplete := false
	defer func() {
		if !bootstrapComplete && resourceCloser != nil {
			_ = resourceCloser.Close()
		}
	}()
	if cfg.RedisEnabled {
		redisClient, redisErr := platformRedis.Connect(context.Background(), cfg.RedisURL)
		if redisErr != nil {
			return nil, fmt.Errorf("configure Redis: %w", redisErr)
		}
		resourceCloser = redisClient
		cacheService = platformCache.NewRedisAdapter(redisClient.Raw(), storeConfig.Code+":")
		loginLimiter = platformRedis.NewFixedWindowLimiter(redisClient.Raw())
	}

	httpDependencies := HTTPDependencies{
		Recovery: gin.Recovery(),
		Timeout:  middleware.TimeoutMiddleware(cfg.RequestTimeout),
		RequestLogging: func(c *gin.Context) {
			logger.Log.Infow("Inbound Request", "method", c.Request.Method, "path", c.Request.URL.Path, "ip", c.ClientIP())
			c.Next()
		},
		CORS:             cors.New(cors.Config{AllowOrigins: cfg.CORSAllowOrigins, AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "Accept-Language"}, AllowCredentials: true}),
		Swagger:          ginSwagger.WrapHandler(swaggerFiles.Handler),
		Health:           func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) },
		LocaleMiddleware: middleware.NewLocaleMiddleware(middleware.LocaleOptions{DefaultLocale: storeConfig.DefaultLocale, FallbackLocale: storeConfig.FallbackLocale, SupportedLocales: storeConfig.SupportedLocales}),
		OptionalAuth:     middleware.OptionalAuthMiddleware(tokenMaker),
		LoginRateLimit:   middleware.LoginRateLimitMiddleware(loginLimiter, cfg.JWTSecret),
	}

	variantService := catalogApp.NewVariantService(catalogPostgres.NewVariantRepository(db), storeConfig.SupportedLocales, storeConfig.Currency)
	inventoryRepository := inventoryPostgres.NewRepository(db)
	inventoryService := inventoryApp.NewService(inventoryMode(storeConfig.InventoryMode), inventoryRepository)
	notificationsEnabled := contains(storeConfig.EnabledModules, "notifications")
	eventConsumers := make([]string, 0, 1)
	if notificationsEnabled {
		eventConsumers = append(eventConsumers, eventsDomain.ConsumerNotifications)
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
	if contains(storeConfig.EnabledModules, "promos") {
		promosRepository := promosPostgres.NewRepository(db)
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

	checkoutService := checkoutApp.NewService(inventoryService, variantService, taxPolicy, checkoutDomain.Policy{
		AllowGuest:                 storeConfig.CheckoutAllowGuest,
		RequirePhone:               storeConfig.CheckoutRequirePhone,
		OrderNumberPrefix:          storeConfig.Code,
		SupportedDeliveryProviders: storeConfig.ShippingProviders,
		DefaultDeliveryProvider:    storeConfig.ShippingDefault,
	}, orderWorkflowService, paymentGateways.Default()).WithCarriers(deliveryCarriers).WithPriceCalculator(priceCalculator)

	categoryService := catalogApp.NewCategoryService(catalogPostgres.NewCategoryRepository(db), storeConfig.SupportedLocales).WithCache(cacheService)
	application := &Application{
		Config: cfg, StoreConfig: storeConfig, TokenMaker: tokenMaker,
		CatalogCategoryService: categoryService,
		CatalogProductService:  productService,
		CatalogVariantService:  variantService,
		CartService:            cartApp.NewService(cartPostgres.NewRepository(db)),
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
		DeliveryDispatcher:     deliveryApp.NewDispatcher(deliveryPostgres.NewJobStore(db), deliveryCarriers, time.Minute),
		DeliveryTracker:        deliveryApp.NewTracker(deliveryPostgres.NewTrackingStore(db), deliveryCarriers),
		OutboxWorker:           eventsApp.NewOutboxWorker(eventsPostgres.NewDeliveryStore(db), eventsDomain.ConsumerNotifications, time.Minute, logger.Log, outboxHandlers...),
		TaxPolicy:              taxPolicy,
		HTTP:                   httpDependencies,
		resourceCloser:         resourceCloser,
	}
	bootstrapComplete = true
	return application, nil
}

func inventoryMode(value string) inventoryDomain.Mode {
	if value == "internal" {
		return inventoryDomain.ModeInternal
	}
	return inventoryDomain.ModeExternal
}

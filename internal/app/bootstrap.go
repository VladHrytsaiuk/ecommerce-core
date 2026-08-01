// Package app assembles application dependencies. It is the Composition Root
// during the incremental migration away from the monolithic HTTP router.
package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	auditDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/audit/domain"
	auditPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/audit/repository/postgres"
	auditSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/audit/service"
	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	cartPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/repository/postgres"
	cartSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/service"
	categoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	categoryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/category/repository/postgres"
	categorySvc "github.com/VladHrytsaiuk/ecommerce-core/internal/category/service"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	discountPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/repository/postgres"
	discountSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/service"
	documentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/document/domain"
	documentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/document/repository/postgres"
	documentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/document/service"
	feedbackDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/domain"
	feedbackPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/repository/postgres"
	feedbackSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/service"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/integration/novaposhta"
	integrationSMS "github.com/VladHrytsaiuk/ecommerce-core/internal/integration/sms"
	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	orderPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/order/repository/postgres"
	orderSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/order/service"
	paymentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	paymentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/repository/postgres"
	paymentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/service"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/notification"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	platformSMS "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/sms"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage/cloudinary"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	productPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/product/repository/postgres"
	productSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/product/service"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
	redirectPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/repository/postgres"
	redirectSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/service"
	shipmentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
	shipmentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/repository/postgres"
	shipmentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/service"
	shippingSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/shipping/service"
	sitemapHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/delivery/http"
	sitemapDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/domain"
	sitemapPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/repository/postgres"
	sitemapSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/service"
	userHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/user/delivery/http"
	userDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	userPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/user/repository/postgres"
	userSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/user/service"
	wishlistDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	wishlistPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/repository/postgres"
	wishlistSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/service"
)

// Application contains the dependency graph consumed by delivery layers.
// It intentionally exposes domain interfaces rather than implementations.
type Application struct {
	Config      *config.Config // Temporary compatibility config for legacy services.
	StoreConfig StoreConfig
	TokenMaker  token.Maker
	WSHub       *notification.Hub

	AuthService       userDomain.AuthService
	UserService       userDomain.UserService
	CategoryService   categoryDomain.CategoryService
	ProductService    productDomain.ProductService
	BrandService      productDomain.BrandService
	AttributeService  productDomain.AttributeService
	BadgeService      productDomain.BadgeService
	RedirectService   redirectDomain.RedirectService
	ShipmentService   shipmentDomain.ShipmentService
	WishlistService   wishlistDomain.WishlistService
	PromoService      discountDomain.PromoService
	CartService       cartDomain.CartService
	AuditService      auditDomain.AuditService
	DocumentService   documentDomain.DocumentService
	PaymentService    paymentDomain.PaymentService
	OrderService      orderDomain.OrderService
	AdminOrderService orderDomain.AdminOrderService
	ManagerService    orderDomain.ManagerService
	FeedbackService   feedbackDomain.FeedbackService
	SitemapWorker     sitemapDomain.SitemapWorkerService

	cleanupWorker         *userSvc.CleanupWorker
	wishlistCleanupWorker *wishlistSvc.WishlistCleanupWorker
	cartCleanupWorker     *cartSvc.CartCleanupWorker
	paymentWorker         *orderSvc.PaymentWorker
	trackingWorker        *orderSvc.TrackingWorker

	workersMu      sync.Mutex
	workersWG      sync.WaitGroup
	workersStarted bool
	stopWorkers    context.CancelFunc

	HTTP HTTPDependencies
}

// HTTPDependencies are constructed in the Composition Root and attached by
// the router without further dependency creation.
type HTTPDependencies struct {
	Recovery               gin.HandlerFunc
	Timeout                gin.HandlerFunc
	RequestLogging         gin.HandlerFunc
	CORS                   gin.HandlerFunc
	Swagger                gin.HandlerFunc
	Health                 gin.HandlerFunc
	AuthHandler            *userHTTP.AuthHandler
	UserHandler            *userHTTP.UserHandler
	SitemapHandler         *sitemapHTTP.SitemapHandler
	AuthMiddleware         gin.HandlerFunc
	OptionalAuthMiddleware gin.HandlerFunc
	SessionMiddleware      gin.HandlerFunc
	OTPSendRateLimit       gin.HandlerFunc
	CustomerRateLimit      gin.HandlerFunc
	AdminRateLimit         gin.HandlerFunc
	FeedbackRateLimit      gin.HandlerFunc
	AdminMiddleware        gin.HandlerFunc
	AuditMiddleware        gin.HandlerFunc
	LocaleMiddleware       gin.HandlerFunc
}

// Bootstrap constructs the current dependency graph. Workers are created here
// but started explicitly by Application.Start. It preserves the legacy
// implementations while moving composition out of the HTTP router.
func Bootstrap(cfg *config.Config, db *gorm.DB, tokenMaker token.Maker) (*Application, error) {
	storeConfig, err := NewStoreConfig(cfg)
	if err != nil {
		return nil, err
	}

	wsHub := notification.NewHub(logger.Log)
	userRepo := userPostgres.NewUserRepository(db, logger.Log)
	sessionRepo := userPostgres.NewSessionRepository(db, logger.Log)
	addressRepo := userPostgres.NewUserAddressRepository(db, logger.Log)
	verifyCodeRepo := userPostgres.NewVerifyCodeRepository(db, logger.Log)

	var emailProvider email.Provider
	if cfg.SendGridAPIKey != "" {
		emailProvider = email.NewSendGridProvider(cfg, logger.Log)
	} else if cfg.SMTPHost != "" && cfg.SMTPUser != "" {
		emailProvider = email.NewSMTPProvider(cfg, logger.Log)
	} else {
		logger.Log.Warn("Email not configured, using console email provider (dev mode)")
		emailProvider = email.NewZapProvider(cfg.StoreLogoURL, logger.Log)
	}

	var smsSender platformSMS.Sender
	if cfg.OBMUsername != "" && cfg.OBMPassword != "" && cfg.OBMSenderID != 0 {
		smsSender = integrationSMS.NewVodafoneSender(integrationSMS.VodafoneConfig{
			BaseURL: cfg.OBMBaseURL, TokenPath: cfg.OBMTokenPath,
			BasicAuthHeader: cfg.OBMBasicAuthHeader, Username: cfg.OBMUsername,
			Password: cfg.OBMPassword, SenderID: cfg.OBMSenderID,
			ValidityMinutes: cfg.OBMValidityMinutes, StatusCheck: cfg.OBMStatusCheck,
		}, logger.Log)
	} else {
		logger.Log.Warn("Vodafone OBM not configured, using console SMS sender (dev mode)")
		smsSender = platformSMS.NewLogSender(logger.Log)
	}

	authService := userSvc.NewAuthService(userRepo, sessionRepo, verifyCodeRepo, emailProvider, smsSender, tokenMaker, wsHub, cfg, logger.Log)
	userService := userSvc.NewUserService(userRepo, addressRepo, sessionRepo, logger.Log)

	var store storage.Storage
	if cfg.CloudinaryURL != "" {
		st, err := cloudinary.New(cfg.CloudinaryURL, logger.Log)
		if err != nil {
			logger.Log.Warnw("failed to initialize cloudinary", "error", err)
		} else {
			store = st
		}
	} else {
		logger.Log.Warn("Cloudinary URL not set, image uploads will fail")
	}

	redirectService := redirectSvc.NewRedirectService(redirectPostgres.NewRedirectRepository(db), logger.Log)
	productRepo := productPostgres.NewProductRepository(db, logger.Log)
	productService := productSvc.NewProductService(productRepo, store, redirectService, cfg, logger.Log)
	categoryService := categorySvc.NewCategoryService(categoryPostgres.NewCategoryRepository(db, logger.Log), productRepo, store, redirectService, logger.Log)
	brandService := productSvc.NewBrandService(productPostgres.NewBrandRepository(db, logger.Log), productRepo, logger.Log)
	attributeService := productSvc.NewAttributeService(productPostgres.NewAttributeRepository(db, logger.Log), logger.Log)
	badgeService := productSvc.NewBadgeService(productPostgres.NewBadgeRepository(db, logger.Log), logger.Log)

	npClient := novaposhta.NewClient(cfg.NovaPoshtaAPIKey, cfg.NovaPoshtaURL, logger.Log)
	shipmentService := shipmentSvc.NewShipmentService(map[string]shipmentDomain.ShipmentProvider{
		"novaposhta": npClient,
	}, shipmentPostgres.NewShippingRuleRepository(db, logger.Log), logger.Log)

	wishlistRepo := wishlistPostgres.NewWishlistRepository(db, logger.Log)
	wishlistService := wishlistSvc.NewWishlistService(wishlistRepo, logger.Log)
	promoRepo := discountPostgres.NewPromoRepository(db, logger.Log)
	promoService := discountSvc.NewPromoService(promoRepo, logger.Log, db)
	cartRepo := cartPostgres.NewCartRepository(db, logger.Log)
	cartService := cartSvc.NewCartService(cartRepo, shipmentService, promoService, logger.Log)
	auditService := auditSvc.NewAuditService(auditPostgres.NewAuditRepository(db), logger.Log)
	documentService := documentSvc.NewDocumentService(documentPostgres.NewDocumentRepository(db, logger.Log), logger.Log)

	orderRepo := orderPostgres.NewOrderRepository(db, logger.Log)
	paymentRepo := paymentPostgres.NewPaymentRepository(db, logger.Log)
	paymentService := paymentSvc.NewPaymentService(paymentRepo, orderRepo, emailProvider, cfg, logger.Log)
	orderService := orderSvc.NewOrderService(orderRepo, cartRepo, userRepo, verifyCodeRepo, promoRepo, promoService, paymentService, shipmentService, emailProvider, cfg, logger.Log)
	feedbackService := feedbackSvc.NewFeedbackService(feedbackPostgres.NewFeedbackRepository(db, logger.Log), emailProvider, store, cfg, logger.Log)

	carrierService := shippingSvc.NewCarrierService(npClient, shipmentService, cfg, logger.Log)
	confirmService := orderSvc.NewConfirmService(orderRepo, carrierService, emailProvider, cfg, logger.Log)
	managerService := orderSvc.NewManagerService(orderRepo, confirmService, logger.Log)
	adminOrderService := orderSvc.NewAdminOrderService(orderRepo, confirmService, paymentService, paymentRepo, logger.Log)
	sitemapWorker := sitemapSvc.NewSitemapWorker(sitemapPostgres.NewSitemapRepository(db), cfg, logger.Log)
	cleanupWorker := userSvc.NewCleanupWorker(sessionRepo, verifyCodeRepo, logger.Log)
	wishlistCleanupWorker := wishlistSvc.NewWishlistCleanupWorker(wishlistRepo, logger.Log)
	cartCleanupWorker := cartSvc.NewCartCleanupWorker(cartRepo, logger.Log)
	paymentWorker := orderSvc.NewPaymentWorker(orderService, logger.Log)
	trackingInterval := time.Duration(cfg.NPTrackingIntervalMinutes) * time.Minute
	trackingWorker := orderSvc.NewTrackingWorker(orderRepo, carrierService, logger.Log, trackingInterval)

	httpDependencies := HTTPDependencies{
		Recovery: gin.Recovery(),
		Timeout:  middleware.TimeoutMiddleware(cfg.RequestTimeout),
		RequestLogging: func(c *gin.Context) {
			logger.Log.Infow("Inbound Request", "method", c.Request.Method, "path", c.Request.URL.Path, "ip", c.ClientIP())
			c.Next()
		},
		CORS: cors.New(cors.Config{
			AllowOrigins: cfg.CORSAllowOrigins, AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:  []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Session-ID", "Accept-Language"},
			ExposeHeaders: []string{"Content-Length", "X-Session-ID"}, AllowCredentials: true,
		}),
		Swagger:                ginSwagger.WrapHandler(swaggerFiles.Handler),
		Health:                 func(c *gin.Context) { c.JSON(200, gin.H{"message": "pong", "status": "API is ready!"}) },
		AuthHandler:            userHTTP.NewAuthHandler(authService, wsHub, cfg, logger.Log),
		UserHandler:            userHTTP.NewUserHandler(userService, logger.Log),
		SitemapHandler:         sitemapHTTP.NewSitemapHandler(sitemapWorker, cfg, logger.Log),
		AuthMiddleware:         middleware.AuthMiddleware(tokenMaker),
		OptionalAuthMiddleware: middleware.OptionalAuthMiddleware(tokenMaker),
		SessionMiddleware:      middleware.SessionMiddleware(strings.HasPrefix(cfg.FrontendURL, "https://")),
		OTPSendRateLimit:       middleware.NewOTPSendRateLimiter(cfg.OTPSendRateLimit, cfg.OTPSendRateInterval).Middleware(),
		CustomerRateLimit:      middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(rate.Limit(5), 10)),
		AdminRateLimit:         middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(rate.Limit(0.2), 3)),
		FeedbackRateLimit:      middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(rate.Every(cfg.EmailRateInterval/time.Duration(cfg.EmailRateLimit)), 3)),
		AdminMiddleware:        middleware.AdminMiddleware(),
		AuditMiddleware:        middleware.AuditMiddleware(auditService),
		LocaleMiddleware: middleware.NewLocaleMiddleware(middleware.LocaleOptions{
			DefaultLocale: storeConfig.DefaultLocale, FallbackLocale: storeConfig.FallbackLocale, SupportedLocales: storeConfig.SupportedLocales,
		}),
	}

	return &Application{
		Config: cfg, StoreConfig: storeConfig, TokenMaker: tokenMaker, WSHub: wsHub,
		AuthService: authService, UserService: userService,
		CategoryService: categoryService, ProductService: productService,
		BrandService: brandService, AttributeService: attributeService,
		BadgeService: badgeService, RedirectService: redirectService,
		ShipmentService: shipmentService, WishlistService: wishlistService,
		PromoService: promoService, CartService: cartService, AuditService: auditService,
		DocumentService: documentService, PaymentService: paymentService,
		OrderService: orderService, AdminOrderService: adminOrderService,
		ManagerService: managerService, FeedbackService: feedbackService,
		SitemapWorker: sitemapWorker,
		cleanupWorker: cleanupWorker, wishlistCleanupWorker: wishlistCleanupWorker,
		cartCleanupWorker: cartCleanupWorker, paymentWorker: paymentWorker,
		trackingWorker: trackingWorker,
		HTTP:           httpDependencies,
	}, nil
}

// Start begins background workers after the application dependency graph is
// fully assembled. It is safe to call more than once.
func (a *Application) Start(ctx context.Context) {
	a.workersMu.Lock()
	if a.workersStarted {
		a.workersMu.Unlock()
		return
	}

	workerCtx, cancel := context.WithCancel(ctx)
	a.workersStarted = true
	a.stopWorkers = cancel
	a.workersMu.Unlock()

	a.startWorker(func() { a.cleanupWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.wishlistCleanupWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.cartCleanupWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.SitemapWorker.Run(workerCtx, 24*time.Hour) })
	a.startWorker(func() { a.paymentWorker.Run(workerCtx, time.Minute) })
	a.startWorker(func() { a.trackingWorker.Run(workerCtx) })
}

// Stop requests worker cancellation and waits without a deadline. Prefer
// StopContext from server shutdown code so a stuck external dependency cannot
// block process termination indefinitely.
func (a *Application) Stop() {
	_ = a.StopContext(context.Background())
}

// StopContext requests worker cancellation and waits until all worker loops
// return or the supplied deadline expires.
func (a *Application) StopContext(ctx context.Context) error {
	a.workersMu.Lock()
	cancel := a.stopWorkers
	a.workersMu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()

	done := make(chan struct{})
	go func() {
		a.workersWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for application workers: %w", ctx.Err())
	}
}

func (a *Application) startWorker(run func()) {
	a.workersWG.Add(1)
	go func() {
		defer a.workersWG.Done()
		run()
	}()
}

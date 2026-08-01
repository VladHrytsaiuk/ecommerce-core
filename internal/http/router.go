package http

import (
	"context"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/time/rate"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/notification"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	platformsms "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/sms"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage/cloudinary"
	userHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/user/delivery/http"
	userPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/user/repository/postgres"
	userService "github.com/VladHrytsaiuk/ecommerce-core/internal/user/service"

	categoryHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/category/delivery/http"
	categoryPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/category/repository/postgres"
	categorySvc "github.com/VladHrytsaiuk/ecommerce-core/internal/category/service"

	productHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/product/delivery/http"
	productPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/product/repository/postgres"
	productSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/product/service"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/integration/novaposhta"
	integrationsms "github.com/VladHrytsaiuk/ecommerce-core/internal/integration/sms"
	shipmentHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/delivery/http"
	shipmentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
	shipmentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/repository/postgres"
	shipmentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/service"

	cartHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/delivery/http"
	cartPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/repository/postgres"
	cartSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/service"

	discountHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/delivery/http"
	discountPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/repository/postgres"
	discountSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/service"

	wishlistHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/delivery/http"
	wishlistPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/repository/postgres"
	wishlistSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/service"

	auditPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/audit/repository/postgres"
	auditSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/audit/service"

	documentHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/document/delivery/http"
	documentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/document/repository/postgres"
	documentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/document/service"

	orderHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/order/delivery/http"
	orderPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/order/repository/postgres"
	orderSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/order/service"

	feedbackHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/delivery/http"
	feedbackPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/repository/postgres"
	feedbackService "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/service"

	paymentHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/delivery/http"
	paymentPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/repository/postgres"
	paymentSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/service"

	shippingSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/shipping/service"

	sitemapHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/delivery/http"
	sitemapPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/repository/postgres"
	sitemapSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/service"

	redirectPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/repository/postgres"
	redirectSvc "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/service"
)

// InitRouter збирає всі роутери додатку до купи
func InitRouter(ctx context.Context, cfg *config.Config, db *gorm.DB, tokenMaker token.Maker) *gin.Engine {
	// 1. Ініціалізація фреймворку
	r := gin.New()

	// Налаштування довірених проксі (для коректного визначення IP за reverse-proxy)
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.Log.Warnw("⚠️ Failed to set trusted proxies", "error", err)
	}

	// 2. Глобальні Middleware
	r.Use(gin.Recovery())
	r.Use(middleware.TimeoutMiddleware(cfg.RequestTimeout))

	// Логування запитів через Zap Logger
	r.Use(func(c *gin.Context) {
		logger.Log.Infow("Inbound Request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"ip", c.ClientIP(),
		)
		c.Next()
	})

	// CORS Middleware configuraton
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Session-ID", "Accept-Language"},
		ExposeHeaders:    []string{"Content-Length", "X-Session-ID"},
		AllowCredentials: true,
	}))

	// 3. Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 4. Ініціалізація модуля User (Dependency Injection)
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
		logger.Log.Warn("⚠️ Email not configured, using console email provider (dev mode)")
		emailProvider = email.NewZapProvider(cfg.StoreLogoURL, logger.Log)
	}

	// SMS-провайдер: реальний Vodafone OBM, якщо налаштовано, інакше консольний мок (dev)
	var smsSender platformsms.Sender
	if cfg.OBMUsername != "" && cfg.OBMPassword != "" && cfg.OBMSenderID != 0 {
		smsSender = integrationsms.NewVodafoneSender(integrationsms.VodafoneConfig{
			BaseURL:         cfg.OBMBaseURL,
			TokenPath:       cfg.OBMTokenPath,
			BasicAuthHeader: cfg.OBMBasicAuthHeader,
			Username:        cfg.OBMUsername,
			Password:        cfg.OBMPassword,
			SenderID:        cfg.OBMSenderID,
			ValidityMinutes: cfg.OBMValidityMinutes,
			StatusCheck:     cfg.OBMStatusCheck,
		}, logger.Log)
	} else {
		logger.Log.Warn("⚠️ Vodafone OBM not configured, using console SMS sender (dev mode)")
		smsSender = platformsms.NewLogSender(logger.Log)
	}

	authServ := userService.NewAuthService(userRepo, sessionRepo, verifyCodeRepo, emailProvider, smsSender, tokenMaker, wsHub, cfg, logger.Log)
	userServ := userService.NewUserService(userRepo, addressRepo, sessionRepo, logger.Log)

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

	redirectRepo := redirectPostgres.NewRedirectRepository(db)
	redirectServ := redirectSvc.NewRedirectService(redirectRepo, logger.Log)

	prodRepo := productPostgres.NewProductRepository(db, logger.Log)
	prodServ := productSvc.NewProductService(prodRepo, store, redirectServ, cfg, logger.Log)

	catRepo := categoryPostgres.NewCategoryRepository(db, logger.Log)
	catServ := categorySvc.NewCategoryService(catRepo, prodRepo, store, redirectServ, logger.Log)

	brandRepo := productPostgres.NewBrandRepository(db, logger.Log)
	brandServ := productSvc.NewBrandService(brandRepo, prodRepo, logger.Log)

	attrRepo := productPostgres.NewAttributeRepository(db, logger.Log)
	attrServ := productSvc.NewAttributeService(attrRepo, logger.Log)

	badgeRepo := productPostgres.NewBadgeRepository(db, logger.Log)
	badgeServ := productSvc.NewBadgeService(badgeRepo, logger.Log)

	// 4.1 Ініціалізація модуля Shipment (провайдери доставки)
	npClient := novaposhta.NewClient(cfg.NovaPoshtaAPIKey, cfg.NovaPoshtaURL, logger.Log)
	shippingProviders := map[string]shipmentDomain.ShipmentProvider{
		"novaposhta": npClient,
		// "ukrposhta": ukrposhtaClient, // Додати пізніше
	}
	shippingRuleRepo := shipmentPostgres.NewShippingRuleRepository(db, logger.Log)
	shipServ := shipmentSvc.NewShipmentService(shippingProviders, shippingRuleRepo, logger.Log)

	// 4.2 Ініціалізація модуля Wishlist
	wishlistRepo := wishlistPostgres.NewWishlistRepository(db, logger.Log)
	wishlistServ := wishlistSvc.NewWishlistService(wishlistRepo, logger.Log)

	// 4.3 Ініціалізація модуля Discount (Promo)
	promoRepo := discountPostgres.NewPromoRepository(db, logger.Log)
	promoServ := discountSvc.NewPromoService(promoRepo, logger.Log, db)

	// 4.4 Ініціалізація модуля Cart
	cartRepo := cartPostgres.NewCartRepository(db, logger.Log)
	cartServ := cartSvc.NewCartService(cartRepo, shipServ, promoServ, logger.Log)

	// 4.5 Ініціалізація модуля Audit
	auditRepo := auditPostgres.NewAuditRepository(db)
	auditService := auditSvc.NewAuditService(auditRepo, logger.Log)

	// 4.6 Ініціалізація модуля Document
	documentRepo := documentPostgres.NewDocumentRepository(db, logger.Log)
	documentServ := documentSvc.NewDocumentService(documentRepo, logger.Log)

	// 4.6 Ініціалізація модуля Payment (має бути до OrderService)
	// orderRepo передається для callback вебхуків (оновлення статусу)
	orderRepo := orderPostgres.NewOrderRepository(db, logger.Log)
	paymentRepo := paymentPostgres.NewPaymentRepository(db, logger.Log)
	paymentServ := paymentSvc.NewPaymentService(paymentRepo, orderRepo, emailProvider, cfg, logger.Log)

	// 4.7 Ініціалізація модуля Order
	orderServ := orderSvc.NewOrderService(orderRepo, cartRepo, userRepo, verifyCodeRepo, promoRepo, promoServ, paymentServ, shipServ, emailProvider, cfg, logger.Log)

	// 4.7.1 Ініціалізація модуля Feedback
	feedbackRepo := feedbackPostgres.NewFeedbackRepository(db, logger.Log)
	feedbackServ := feedbackService.NewFeedbackService(feedbackRepo, emailProvider, store, cfg, logger.Log)

	// 4.8 Ініціалізація модуля Shipping (CarrierService для створення ТТН)
	carrierServ := shippingSvc.NewCarrierService(npClient, shipServ, cfg, logger.Log)

	// 4.9 Ініціалізація Order Business Logic Services
	confirmServ := orderSvc.NewConfirmService(orderRepo, carrierServ, emailProvider, cfg, logger.Log)
	managerServ := orderSvc.NewManagerService(orderRepo, confirmServ, logger.Log)
	adminSvc := orderSvc.NewAdminOrderService(orderRepo, confirmServ, paymentServ, paymentRepo, logger.Log)

	// 4.10 Ініціалізація Sitemap
	sitemapRepo := sitemapPostgres.NewSitemapRepository(db)
	sitemapWorker := sitemapSvc.NewSitemapWorker(sitemapRepo, cfg, logger.Log)

	// 5. Запуск фонових воркерів (Background Workers)
	cleanupWorker := userService.NewCleanupWorker(sessionRepo, verifyCodeRepo, logger.Log)
	// Використовуємо переданий ctx для коректної зупинки при завершенні додатка
	cleanupWorker.Start(ctx, 24*time.Hour)

	// Воркер для очищення застарілих анонімних записів вішліста (кожні 24 години)
	wishlistCleanup := wishlistSvc.NewWishlistCleanupWorker(wishlistRepo, logger.Log)
	wishlistCleanup.Start(ctx, 24*time.Hour)

	// Воркер для очищення застарілих анонімних кошиків (кожні 24 години)
	cartCleanup := cartSvc.NewCartCleanupWorker(cartRepo, logger.Log)
	cartCleanup.Start(ctx, 24*time.Hour)

	// Воркер для генерації Sitemap (раз на добу)
	sitemapWorker.Start(ctx, 24*time.Hour)

	// Воркер для перевірки тайм-аутів оплати (кожні 1 хвилину)
	paymentWorker := orderSvc.NewPaymentWorker(orderServ, logger.Log)
	paymentWorker.Start(ctx, 1*time.Minute)

	// 4.10 Фоновий воркер для трекінгу НП
	trackingInterval := time.Duration(cfg.NPTrackingIntervalMinutes) * time.Minute
	trackingWorker := orderSvc.NewTrackingWorker(orderRepo, carrierServ, logger.Log, trackingInterval)
	go trackingWorker.Run(ctx)

	authHandler := userHttp.NewAuthHandler(authServ, wsHub, cfg, logger.Log)
	userHandler := userHttp.NewUserHandler(userServ, logger.Log)

	authMiddleware := middleware.AuthMiddleware(tokenMaker)
	optionalAuthMiddleware := middleware.OptionalAuthMiddleware(tokenMaker)
	// Визначаємо Secure-прапорець для cookie на основі конфігурації (HTTPS = secure)
	cookieSecure := strings.HasPrefix(cfg.FrontendURL, "https://")
	sessionMiddleware := middleware.SessionMiddleware(cookieSecure)
	otpSendLimiter := middleware.NewOTPSendRateLimiter(cfg.OTPSendRateLimit, cfg.OTPSendRateInterval)
	otpSendRateLimitMiddleware := otpSendLimiter.Middleware()

	// Ініціалізуємо Rate Limiters
	// Клієнти: 5 запитів на секунду, burst 10
	customerLimiter := middleware.NewIPRateLimiter(rate.Limit(5), 10)
	customerRateLimitMiddleware := middleware.RateLimitMiddleware(customerLimiter)

	// Адмінка: 1 запит на 5 секунд (0.2), burst 3 (жорстке обмеження для захисту від перебору)
	adminLimiter := middleware.NewIPRateLimiter(rate.Limit(0.2), 3)
	adminRateLimitMiddleware := middleware.RateLimitMiddleware(adminLimiter)

	// Лімітер відгуків: максимум 3 запити, відновлення 1 токену кожні 30 секунд (EMAIL_RATE_INTERVAL/EMAIL_RATE_LIMIT)
	feedbackLimiter := middleware.NewIPRateLimiter(rate.Every(cfg.EmailRateInterval/time.Duration(cfg.EmailRateLimit)), 3)
	feedbackRateLimitMiddleware := middleware.RateLimitMiddleware(feedbackLimiter)

	// 6. Групування роутів
	api := r.Group("/api")
	{

		// health check
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong", "status": "API is ready!"})
		})

		adminApiGroup := api.Group("/admin")

		// Підключення модуля Sitemap (публічні маршрути)
		sitemapHandler := sitemapHttp.NewSitemapHandler(sitemapWorker, cfg, logger.Log)
		sitemapHttp.RegisterSitemapRoutes(api, sitemapHandler)

		// Підключення модуля User/Auth
		userHttp.RegisterRoutes(api, adminApiGroup, authHandler, userHandler, authMiddleware, optionalAuthMiddleware, otpSendRateLimitMiddleware, customerRateLimitMiddleware, adminRateLimitMiddleware)

		// Групи для захищених маршрутів (Catalog)
		authGrp := api.Group("")
		authGrp.Use(authMiddleware)

		adminGrp := api.Group("/admin")
		adminGrp.Use(authMiddleware, middleware.AdminMiddleware(), middleware.AuditMiddleware(auditService))

		// Підключення модуля Shipment (Nova Poshta)
		shipmentHttp.RegisterShipmentRoutes(api, adminGrp, shipServ, logger.Log)

		discountHttp.RegisterPromoRoutes(adminGrp, promoServ, logger.Log)

		// Підключення модуля Feedback
		feedbackHttp.RegisterFeedbackRoutes(api, adminGrp, feedbackRateLimitMiddleware, feedbackServ, logger.Log)

		langGroup := api.Group("/:lang")
		langGroup.Use(middleware.LocaleMiddleware())
		{
			categoryHttp.RegisterCategoryRoutes(langGroup, adminGrp, catServ, redirectServ, logger.Log)
			productHttp.RegisterBrandRoutes(langGroup, adminGrp, brandServ, logger.Log)
			productHttp.RegisterAttributeRoutes(langGroup, adminGrp, attrServ, logger.Log)
			productHttp.RegisterBadgeRoutes(adminGrp, badgeServ, logger.Log)

			// Захищена група всередині мовної групи для авторизованих маршрутів
			authLangGrp := langGroup.Group("")
			authLangGrp.Use(authMiddleware)

			// Група з опціональною авторизацією та сесійною кукою (для вішліста — працює і для анонімних)
			optionalAuthLangGrp := langGroup.Group("")
			optionalAuthLangGrp.Use(optionalAuthMiddleware, sessionMiddleware)

			// Реєструємо всі маршрути продуктів
			productHttp.RegisterProductRoutes(langGroup, authLangGrp, adminGrp, api, authGrp, prodServ, catServ, redirectServ, logger.Log)

			// Реєструємо маршрути вішліста
			wishlistHttp.RegisterWishlistRoutes(optionalAuthLangGrp, authLangGrp, wishlistServ, logger.Log)

			// Реєструємо маршрути кошика
			cartHttp.RegisterCartRoutes(optionalAuthLangGrp, authLangGrp, cartServ, logger.Log)

			// Реєструємо маршрути замовлень
			orderHttp.RegisterOrderRoutes(optionalAuthLangGrp, authLangGrp, orderServ, logger.Log)
			// Реєструємо маршрути адмінки для замовлень
			orderHttp.RegisterAdminOrderRoutes(adminGrp, adminSvc, logger.Log)

			// Реєструємо маршрути документів
			documentHttp.RegisterDocumentRoutes(langGroup, adminGrp, documentServ, logger.Log)
		}

		// Webhook group — без авторизації, з валідацією підписом
		webhookGrp := api.Group("/webhooks")
		paymentHttp.RegisterWebhookRoutes(webhookGrp, paymentServ, logger.Log)
	}

	// Manager routes (на root engine, не під /api групою — включають HTML page)
	orderHttp.RegisterManagerRoutes(r, managerServ, logger.Log)

	return r
}

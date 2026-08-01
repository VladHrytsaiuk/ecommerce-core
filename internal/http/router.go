package http

import (
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/time/rate"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	cartHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/delivery/http"
	categoryHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/category/delivery/http"
	discountHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/delivery/http"
	documentHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/document/delivery/http"
	feedbackHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	orderHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/order/delivery/http"
	paymentHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/product/delivery/http"
	shipmentHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/delivery/http"
	sitemapHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/delivery/http"
	userHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/user/delivery/http"
	wishlistHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/delivery/http"
)

// InitRouter attaches HTTP handlers to a pre-built application graph.
// Dependency construction belongs to internal/app.Bootstrap.
func InitRouter(application *app.Application) *gin.Engine {
	cfg := application.Config
	r := gin.New()

	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.Log.Warnw("failed to set trusted proxies", "error", err)
	}

	r.Use(gin.Recovery())
	r.Use(middleware.TimeoutMiddleware(cfg.RequestTimeout))
	r.Use(func(c *gin.Context) {
		logger.Log.Infow("Inbound Request", "method", c.Request.Method, "path", c.Request.URL.Path, "ip", c.ClientIP())
		c.Next()
	})
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Session-ID", "Accept-Language"},
		ExposeHeaders:    []string{"Content-Length", "X-Session-ID"},
		AllowCredentials: true,
	}))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	authHandler := userHTTP.NewAuthHandler(application.AuthService, application.WSHub, cfg, logger.Log)
	userHandler := userHTTP.NewUserHandler(application.UserService, logger.Log)
	authMiddleware := middleware.AuthMiddleware(application.TokenMaker)
	optionalAuthMiddleware := middleware.OptionalAuthMiddleware(application.TokenMaker)
	sessionMiddleware := middleware.SessionMiddleware(strings.HasPrefix(cfg.FrontendURL, "https://"))
	otpSendRateLimitMiddleware := middleware.NewOTPSendRateLimiter(cfg.OTPSendRateLimit, cfg.OTPSendRateInterval).Middleware()
	customerRateLimitMiddleware := middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(rate.Limit(5), 10))
	adminRateLimitMiddleware := middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(rate.Limit(0.2), 3))
	feedbackRateLimitMiddleware := middleware.RateLimitMiddleware(
		middleware.NewIPRateLimiter(rate.Every(cfg.EmailRateInterval/time.Duration(cfg.EmailRateLimit)), 3),
	)

	api := r.Group("/api")
	api.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong", "status": "API is ready!"})
	})

	adminAPIGroup := api.Group("/admin")
	sitemapHandler := sitemapHTTP.NewSitemapHandler(application.SitemapWorker, cfg, logger.Log)
	sitemapHTTP.RegisterSitemapRoutes(api, sitemapHandler)
	userHTTP.RegisterRoutes(api, adminAPIGroup, authHandler, userHandler, authMiddleware, optionalAuthMiddleware, otpSendRateLimitMiddleware, customerRateLimitMiddleware, adminRateLimitMiddleware)

	authGroup := api.Group("")
	authGroup.Use(authMiddleware)
	adminGroup := api.Group("/admin")
	adminGroup.Use(authMiddleware, middleware.AdminMiddleware(), middleware.AuditMiddleware(application.AuditService))

	shipmentHTTP.RegisterShipmentRoutes(api, adminGroup, application.ShipmentService, logger.Log)
	discountHTTP.RegisterPromoRoutes(adminGroup, application.PromoService, logger.Log)
	feedbackHTTP.RegisterFeedbackRoutes(api, adminGroup, feedbackRateLimitMiddleware, application.FeedbackService, logger.Log)

	localeGroup := api.Group("/:lang")
	localeGroup.Use(middleware.LocaleMiddleware())
	{
		categoryHTTP.RegisterCategoryRoutes(localeGroup, adminGroup, application.CategoryService, application.RedirectService, logger.Log)
		productHTTP.RegisterBrandRoutes(localeGroup, adminGroup, application.BrandService, logger.Log)
		productHTTP.RegisterAttributeRoutes(localeGroup, adminGroup, application.AttributeService, logger.Log)
		productHTTP.RegisterBadgeRoutes(adminGroup, application.BadgeService, logger.Log)

		authLocaleGroup := localeGroup.Group("")
		authLocaleGroup.Use(authMiddleware)
		optionalAuthLocaleGroup := localeGroup.Group("")
		optionalAuthLocaleGroup.Use(optionalAuthMiddleware, sessionMiddleware)

		productHTTP.RegisterProductRoutes(localeGroup, authLocaleGroup, adminGroup, api, authGroup, application.ProductService, application.CategoryService, application.RedirectService, logger.Log)
		wishlistHTTP.RegisterWishlistRoutes(optionalAuthLocaleGroup, authLocaleGroup, application.WishlistService, logger.Log)
		cartHTTP.RegisterCartRoutes(optionalAuthLocaleGroup, authLocaleGroup, application.CartService, logger.Log)
		orderHTTP.RegisterOrderRoutes(optionalAuthLocaleGroup, authLocaleGroup, application.OrderService, logger.Log)
		orderHTTP.RegisterAdminOrderRoutes(adminGroup, application.AdminOrderService, logger.Log)
		documentHTTP.RegisterDocumentRoutes(localeGroup, adminGroup, application.DocumentService, logger.Log)
	}

	webhookGroup := api.Group("/webhooks")
	paymentHTTP.RegisterWebhookRoutes(webhookGroup, application.PaymentService, logger.Log)
	orderHTTP.RegisterManagerRoutes(r, application.ManagerService, logger.Log)

	return r
}

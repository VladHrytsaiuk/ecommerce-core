package http

import (
	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	cartHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/delivery/http"
	catalogHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/delivery/http"
	categoryHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/category/delivery/http"
	discountHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/delivery/http"
	documentHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/document/delivery/http"
	feedbackHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/delivery/http"
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

	r.Use(application.HTTP.Recovery, application.HTTP.Timeout, application.HTTP.RequestLogging, application.HTTP.CORS)
	r.GET("/swagger/*any", application.HTTP.Swagger)

	api := r.Group("/api")
	api.GET("/ping", application.HTTP.Health)

	adminAPIGroup := api.Group("/admin")
	sitemapHTTP.RegisterSitemapRoutes(api, application.HTTP.SitemapHandler)
	userHTTP.RegisterRoutes(api, adminAPIGroup, application.HTTP.AuthHandler, application.HTTP.UserHandler, application.HTTP.AuthMiddleware, application.HTTP.OptionalAuthMiddleware, application.HTTP.OTPSendRateLimit, application.HTTP.CustomerRateLimit, application.HTTP.AdminRateLimit)

	authGroup := api.Group("")
	authGroup.Use(application.HTTP.AuthMiddleware)
	adminGroup := api.Group("/admin")
	adminGroup.Use(application.HTTP.AuthMiddleware, application.HTTP.AdminMiddleware, application.HTTP.AuditMiddleware)

	shipmentHTTP.RegisterShipmentRoutes(api, adminGroup, application.ShipmentService, logger.Log)
	discountHTTP.RegisterPromoRoutes(adminGroup, application.PromoService, logger.Log)
	feedbackHTTP.RegisterFeedbackRoutes(api, adminGroup, application.HTTP.FeedbackRateLimit, application.FeedbackService, logger.Log)

	localeGroup := api.Group("/:lang")
	localeGroup.Use(application.HTTP.LocaleMiddleware)
	{
		catalogHTTP.RegisterCategoryRoutes(localeGroup, adminGroup, application.CatalogCategoryService)
		catalogHTTP.RegisterProductRoutes(localeGroup, adminGroup, application.CatalogProductService)
		categoryHTTP.RegisterCategoryRoutes(localeGroup, adminGroup, application.CategoryService, application.RedirectService, logger.Log)
		productHTTP.RegisterBrandRoutes(localeGroup, adminGroup, application.BrandService, logger.Log)
		productHTTP.RegisterAttributeRoutes(localeGroup, adminGroup, application.AttributeService, logger.Log)
		productHTTP.RegisterBadgeRoutes(adminGroup, application.BadgeService, logger.Log)

		authLocaleGroup := localeGroup.Group("")
		authLocaleGroup.Use(application.HTTP.AuthMiddleware)
		optionalAuthLocaleGroup := localeGroup.Group("")
		optionalAuthLocaleGroup.Use(application.HTTP.OptionalAuthMiddleware, application.HTTP.SessionMiddleware)

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

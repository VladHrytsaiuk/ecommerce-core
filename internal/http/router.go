package http

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	adminHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	availabilityHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/delivery/http"
	cartHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/delivery/http"
	catalogHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/delivery/http"
	checkoutHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/delivery/http"
	comparisonHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/delivery/http"
	deliveryHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/delivery/http"
	mediaHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/media/delivery/http"
	ordersHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/delivery/http"
	paymentsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	reportsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/delivery/http"
	returnsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/delivery/http"
	searchHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/search/delivery/http"
	wishlistHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/delivery/http"
)

// InitRouter only attaches handlers assembled by app.Bootstrap.
func InitRouter(application *app.Application) *gin.Engine {
	r := gin.New()
	if err := r.SetTrustedProxies(application.Config.TrustedProxies); err != nil {
		logger.Log.Warnw("failed to set trusted proxies", "error", err)
	}
	r.Use(application.HTTP.Recovery, application.HTTP.Observability, application.HTTP.Timeout, application.HTTP.RequestLogging, application.HTTP.CORS)
	r.GET("/swagger/*any", application.HTTP.Swagger)

	api := r.Group("/api")
	api.Use(middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(perMinute(application.Config.APIRateLimitPerMin), application.Config.APIRateLimitPerMin)))
	api.GET("/ping", application.HTTP.Health)
	sensitiveLimit := middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(perMinute(application.Config.SensitiveRatePerMin), application.Config.SensitiveRatePerMin))
	oauthRedirectURI := ""
	if application.StoreConfig.GoogleOAuth != nil {
		oauthRedirectURI = application.StoreConfig.GoogleOAuth.RedirectURI
	}
	identityHTTP.RegisterRoutes(api, application.IdentityAuthService, application.IdentityProfileService, oauthRedirectURI, middleware.AuthMiddleware(application.TokenMaker), sensitiveLimit, application.HTTP.LoginRateLimit)
	if application.WishlistService != nil {
		wishlist := api.Group("/wishlist")
		wishlist.Use(application.HTTP.OptionalAuth)
		wishlistHTTP.RegisterRoutes(wishlist, application.WishlistService, application.Config.CookieSecure)
	}
	if application.ComparisonService != nil {
		comparison := api.Group("/comparison")
		comparison.Use(application.HTTP.OptionalAuth)
		comparisonHTTP.RegisterRoutes(comparison, application.ComparisonService, application.Config.CookieSecure)
	}
	if application.PaymentGateways != nil && application.PaymentGateways.Default() != nil {
		paymentsHTTP.RegisterWebhookRoutes(api, application.PaymentWebhookService)
	}
	v1 := api.Group("/v1")
	v1.Use(application.HTTP.SecurityHeaders, application.HTTP.ErrorRenderer.Middleware(), application.HTTP.APIRateLimit)
	identityHTTP.RegisterV1CustomerRoutes(v1, application.CustomerProfileService, middleware.AuthMiddleware(application.TokenMaker), application.HTTP.ErrorRenderer)
	returnsHTTP.RegisterV1CustomerRoutes(v1, application.ReturnService, middleware.AuthMiddleware(application.TokenMaker), application.HTTP.ErrorRenderer)
	deliveryHTTP.RegisterV1LocationRoutes(v1.Group("/delivery"), application.DeliveryLocations, application.HTTP.ErrorRenderer)
	v1Catalog := v1.Group("/catalog/:lang")
	v1Catalog.Use(application.HTTP.RequestBodyLimit, application.HTTP.LocaleMiddleware)
	catalogHTTP.RegisterV1Routes(v1Catalog, application.CatalogProductService, application.HTTP.ErrorRenderer, application.InventoryAvailability)
	availabilityHTTP.RegisterV1Routes(v1, application.AvailabilityService, application.HTTP.ErrorRenderer, application.HTTP.OptionalAuth)
	if application.SearchService != nil {
		searchHTTP.RegisterV1Routes(v1Catalog, application.SearchService, application.HTTP.ErrorRenderer)
	}
	v1Checkout := v1.Group("/checkout/:lang")
	v1Checkout.Use(application.HTTP.RequestBodyLimit, application.HTTP.LocaleMiddleware, application.HTTP.OptionalAuth, sensitiveLimit)
	checkoutHTTP.RegisterV1Routes(v1Checkout, application.CheckoutService, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID, application.Config.CookieSecure, application.HTTP.ErrorRenderer)
	v1Admin := v1.Group("/admin")
	v1Admin.Use(application.HTTP.RequestBodyLimit, middleware.AuthMiddleware(application.TokenMaker))
	adminHTTP.RegisterV1Routes(v1Admin, application.AdminAuthorizer, application.PromosAdminFacade, application.CatalogAdminFacade, application.OrdersAdminFacade, application.HTTP.ErrorRenderer)
	returnsHTTP.RegisterV1AdminRoutes(v1Admin, application.AdminAuthorizer, application.ReturnService, application.HTTP.ErrorRenderer)
	reportsHTTP.RegisterV1Routes(v1Admin, application.AdminAuthorizer, application.ReportsQueryService, application.ReportsRebuilder, application.HTTP.ErrorRenderer)
	if application.MediaUploadService != nil {
		v1AdminMedia := v1.Group("/admin/media")
		v1AdminMedia.Use(application.HTTP.MediaRequestBodyLimit, middleware.AuthMiddleware(application.TokenMaker))
		mediaHTTP.RegisterV1Routes(v1AdminMedia, application.AdminAuthorizer, application.MediaUploadService, application.HTTP.ErrorRenderer)
	}
	v1Orders := v1.Group("/orders")
	v1Orders.Use(application.HTTP.RequestBodyLimit, middleware.AuthMiddleware(application.TokenMaker))
	ordersHTTP.RegisterV1Routes(v1Orders, application.OrderService, application.HTTP.ErrorRenderer)
	admin := api.Group("/admin")
	admin.Use(middleware.AuthMiddleware(application.TokenMaker))
	// Reviews, SEO, Badges and Variant mutations are intentionally not exposed
	// here until each has an Admin Facade that appends an audit event in the
	// same transaction. Leaving permission-protected but unaudited routes live
	// would create a forensic bypass.
	// New Admin facades use data-driven RBAC. Existing legacy admin routes keep
	// their compatibility middleware until they are migrated individually.
	if application.PromosAdminFacade != nil {
		adminHTTP.RegisterPromosRoutes(admin, application.AdminAuthorizer, application.PromosAdminFacade)
	}
	if application.CatalogAdminFacade != nil {
		adminHTTP.RegisterCatalogRoutes(admin, application.AdminAuthorizer, application.CatalogAdminFacade)
	}
	if application.OrdersAdminFacade != nil {
		adminHTTP.RegisterOrdersRoutes(admin, application.AdminAuthorizer, application.OrdersAdminFacade)
	}
	localized := api.Group("/:lang")
	localized.Use(application.HTTP.LocaleMiddleware)
	cart := localized.Group("")
	cart.Use(application.HTTP.OptionalAuth)
	cartHTTP.RegisterRoutes(cart, application.CartService, application.Config.CookieSecure)
	checkoutHTTP.RegisterRoutes(cart, application.CheckoutService, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID, application.Config.CookieSecure, sensitiveLimit)
	catalogHTTP.RegisterCategoryRoutes(localized, nil, application.CatalogCategoryService)
	catalogHTTP.RegisterProductRoutes(localized, nil, application.CatalogProductService)
	return r
}

func perMinute(limit int) rate.Limit {
	return rate.Limit(float64(limit) / 60)
}

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
	consentHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/consent/delivery/http"
	deliveryHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/delivery/http"
	mediaHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/media/delivery/http"
	ordersHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/delivery/http"
	paymentsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/delivery/http"
	reportsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/reports/delivery/http"
	returnsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/delivery/http"
	reviewsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/delivery/http"
	searchHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/search/delivery/http"
	supportHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/support/delivery/http"
	videoHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/video/delivery/http"
	wishlistHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/delivery/http"
)

// InitRouter only attaches handlers assembled by app.Bootstrap.
func InitRouter(application *app.Application) *gin.Engine {
	if application.Config.Env == "production" {
		// Gin defaults to debug mode, which dumps every registered route at
		// startup and keeps its diagnostic output on. Neither belongs in a
		// production log stream.
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	if err := r.SetTrustedProxies(application.Config.TrustedProxies); err != nil {
		// Config validates this input before composition. Do not continue with
		// Gin's proxy defaults if that invariant is ever violated: doing so
		// would make IP-scoped protection depend on forged forwarded headers.
		panic("validated trusted proxy configuration rejected by Gin: " + err.Error())
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
	if application.VideoWebhookService != nil {
		videoHTTP.RegisterWebhookRoutes(v1, application.VideoWebhookService, application.HTTP.ErrorRenderer)
	}
	identityHTTP.RegisterV1CustomerRoutes(v1, application.CustomerProfileService, middleware.AuthMiddleware(application.TokenMaker), application.HTTP.ErrorRenderer)
	returnsHTTP.RegisterV1CustomerRoutes(v1, application.ReturnService, middleware.AuthMiddleware(application.TokenMaker), application.HTTP.ErrorRenderer)
	deliveryHTTP.RegisterV1LocationRoutes(v1.Group("/delivery"), application.DeliveryLocations, application.HTTP.ErrorRenderer)
	v1Catalog := v1.Group("/catalog/:lang")
	v1Catalog.Use(application.HTTP.RequestBodyLimit, application.HTTP.LocaleMiddleware)
	catalogHTTP.RegisterV1Routes(v1Catalog, application.CatalogProductService, application.CatalogCategoryService, application.HTTP.ErrorRenderer, application.InventoryAvailability)
	if application.VideoStorefront != nil {
		videoHTTP.RegisterStorefrontRoutes(v1.Group("/catalog/products"), application.VideoStorefront, application.HTTP.ErrorRenderer)
	}
	availabilityHTTP.RegisterV1Routes(v1, application.AvailabilityService, application.HTTP.ErrorRenderer, application.HTTP.OptionalAuth)
	// Reviews had admin moderation and a Catalog rating projection but no way
	// for a customer to write or read one, so the module was enabled and
	// unusable. Moderation stays with ContentAdminFacade; only the public
	// surface is registered here.
	reviewsHTTP.RegisterV1Routes(v1, application.ReviewsService, middleware.AuthMiddleware(application.TokenMaker), application.HTTP.ErrorRenderer)
	supportHTTP.RegisterV1Routes(v1, application.SupportService, application.HTTP.ErrorRenderer, application.HTTP.OptionalAuth, middleware.AuthMiddleware(application.TokenMaker))
	consentHTTP.RegisterV1Routes(v1, application.ConsentService, middleware.AuthMiddleware(application.TokenMaker), application.HTTP.ErrorRenderer)
	if application.SearchService != nil {
		searchHTTP.RegisterV1Routes(v1Catalog, application.SearchService, application.HTTP.ErrorRenderer)
	}
	v1Checkout := v1.Group("/checkout/:lang")
	v1Checkout.Use(application.HTTP.RequestBodyLimit, application.HTTP.LocaleMiddleware, application.HTTP.OptionalAuth, sensitiveLimit)
	checkoutHTTP.RegisterV1Routes(v1Checkout, application.CheckoutService, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID, application.Config.CookieSecure, application.HTTP.ErrorRenderer)
	v1CheckoutContact := v1.Group("/checkout")
	v1CheckoutContact.Use(application.HTTP.RequestBodyLimit, application.HTTP.OptionalAuth, application.HTTP.LoginRateLimit, sensitiveLimit)
	checkoutHTTP.RegisterV1ContactRoute(v1CheckoutContact, application.CheckoutService, application.CheckoutContactCapture, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID, application.Config.CookieSecure, application.HTTP.ErrorRenderer)
	v1Admin := v1.Group("/admin")
	v1Admin.Use(application.HTTP.RequestBodyLimit, middleware.AuthMiddleware(application.TokenMaker))
	adminHTTP.RegisterV1Routes(v1Admin, application.AdminAuthorizer, application.PromosAdminFacade, application.CatalogAdminFacade, application.OrdersAdminFacade, application.HTTP.ErrorRenderer)
	returnsHTTP.RegisterV1AdminRoutes(v1Admin, application.AdminAuthorizer, application.ReturnService, application.HTTP.ErrorRenderer)
	reportsHTTP.RegisterV1Routes(v1Admin, application.AdminAuthorizer, application.ReportsQueryService, application.ReportsRebuilder, application.HTTP.ErrorRenderer)
	supportHTTP.RegisterV1AdminRoutes(v1Admin, application.AdminAuthorizer, application.SupportService, application.HTTP.ErrorRenderer)
	consentHTTP.RegisterV1AdminRoutes(v1Admin, application.AdminAuthorizer, application.ConsentService, application.HTTP.ErrorRenderer)
	adminHTTP.RegisterV1ContentRoutes(v1Admin, application.AdminAuthorizer, application.ContentAdminFacade, application.HTTP.ErrorRenderer)
	if application.MediaUploadService != nil {
		v1AdminMedia := v1.Group("/admin/media")
		v1AdminMedia.Use(application.HTTP.MediaRequestBodyLimit, middleware.AuthMiddleware(application.TokenMaker))
		mediaHTTP.RegisterV1Routes(v1AdminMedia, application.AdminAuthorizer, application.MediaUploadService, application.HTTP.ErrorRenderer)
	}
	if application.VideoUploadService != nil {
		videoHTTP.RegisterV1Routes(v1Admin, application.AdminAuthorizer, application.VideoUploadService, application.HTTP.ErrorRenderer)
		videoHTTP.RegisterV1PlacementRoutes(v1Admin, application.AdminAuthorizer, application.VideoPlacementFacade, application.HTTP.ErrorRenderer)
	}
	v1Orders := v1.Group("/orders")
	v1Orders.Use(application.HTTP.RequestBodyLimit, middleware.AuthMiddleware(application.TokenMaker))
	ordersHTTP.RegisterV1Routes(v1Orders, application.OrderService, application.HTTP.ErrorRenderer)
	// The whole admin surface lives under /v1/admin. A second registration at
	// /api/admin served the same operations through separate handlers, and
	// promised in a comment to be migrated route by route; that never
	// happened, and there are no existing clients here to migrate — this core
	// is copied to start a store. v1 covered every one of those routes, so
	// removing them takes nothing with it.
	localized := api.Group("/:lang")
	localized.Use(application.HTTP.LocaleMiddleware)
	cart := localized.Group("")
	cart.Use(application.HTTP.OptionalAuth)
	cartHTTP.RegisterRoutes(cart, application.CartService, application.Config.CookieSecure)
	checkoutHTTP.RegisterRoutes(cart, application.CheckoutService, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID, application.Config.CookieSecure, sensitiveLimit)
	return r
}

func perMinute(limit int) rate.Limit {
	return rate.Limit(float64(limit) / 60)
}

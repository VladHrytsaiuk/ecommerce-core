package http

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	badgesHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/delivery/http"
	cartHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/delivery/http"
	catalogHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/delivery/http"
	checkoutHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/delivery/http"
	comparisonHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	identityHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/identity/delivery/http"
	paymentsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	reviewsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/delivery/http"
	seoHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/delivery/http"
	wishlistHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/delivery/http"
)

// InitRouter only attaches handlers assembled by app.Bootstrap.
func InitRouter(application *app.Application) *gin.Engine {
	r := gin.New()
	if err := r.SetTrustedProxies(application.Config.TrustedProxies); err != nil {
		logger.Log.Warnw("failed to set trusted proxies", "error", err)
	}
	r.Use(application.HTTP.Recovery, application.HTTP.Timeout, application.HTTP.RequestLogging, application.HTTP.CORS)
	r.GET("/swagger/*any", application.HTTP.Swagger)

	api := r.Group("/api")
	api.Use(middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(perMinute(application.Config.APIRateLimitPerMin), application.Config.APIRateLimitPerMin)))
	api.GET("/ping", application.HTTP.Health)
	sensitiveLimit := middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(perMinute(application.Config.SensitiveRatePerMin), application.Config.SensitiveRatePerMin))
	oauthRedirectURI := ""
	if application.StoreConfig.GoogleOAuth != nil {
		oauthRedirectURI = application.StoreConfig.GoogleOAuth.RedirectURI
	}
	identityHTTP.RegisterRoutes(api, application.IdentityAuthService, application.IdentityProfileService, oauthRedirectURI, middleware.AuthMiddleware(application.TokenMaker), sensitiveLimit)
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
	admin := api.Group("/admin")
	admin.Use(middleware.AuthMiddleware(application.TokenMaker), middleware.AdminMiddleware())
	if application.ReviewsService != nil {
		reviewsHTTP.RegisterRoutes(api, admin, application.ReviewsService, middleware.AuthMiddleware(application.TokenMaker))
	}
	if application.SEOService != nil {
		seoHTTP.RegisterRoutes(admin, application.SEOService)
	}
	if application.BadgesService != nil {
		badgesHTTP.RegisterRoutes(admin, application.BadgesService)
	}
	localized := api.Group("/:lang")
	localized.Use(application.HTTP.LocaleMiddleware)
	cart := localized.Group("")
	cart.Use(application.HTTP.OptionalAuth)
	cartHTTP.RegisterRoutes(cart, application.CartService, application.Config.CookieSecure)
	checkoutHTTP.RegisterRoutes(cart, application.CheckoutService, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID, application.Config.CookieSecure, sensitiveLimit)
	catalogHTTP.RegisterCategoryRoutes(localized, admin, application.CatalogCategoryService)
	catalogHTTP.RegisterProductRoutes(localized, admin, application.CatalogProductService)
	catalogHTTP.RegisterVariantRoutes(admin, application.CatalogVariantService)
	return r
}

func perMinute(limit int) rate.Limit {
	return rate.Limit(float64(limit) / 60)
}

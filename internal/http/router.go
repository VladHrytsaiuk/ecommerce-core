package http

import (
	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	cartHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/delivery/http"
	catalogHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/delivery/http"
	checkoutHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/delivery/http"
	paymentsHTTP "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
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
	api.GET("/ping", application.HTTP.Health)
	if application.PaymentGateways != nil && application.PaymentGateways.Default() != nil {
		paymentsHTTP.RegisterWebhookRoutes(api, application.PaymentWebhookService)
	}
	admin := api.Group("/admin")
	localized := api.Group("/:lang")
	localized.Use(application.HTTP.LocaleMiddleware)
	cart := localized.Group("")
	cart.Use(application.HTTP.OptionalAuth)
	cartHTTP.RegisterRoutes(cart, application.CartService)
	checkoutHTTP.RegisterRoutes(cart, application.CheckoutService, application.CartService, application.StoreConfig.CheckoutReservationTTL, application.StoreConfig.DefaultWarehouseID)
	catalogHTTP.RegisterCategoryRoutes(localized, admin, application.CatalogCategoryService)
	catalogHTTP.RegisterProductRoutes(localized, admin, application.CatalogProductService)
	catalogHTTP.RegisterVariantRoutes(admin, application.CatalogVariantService)
	return r
}

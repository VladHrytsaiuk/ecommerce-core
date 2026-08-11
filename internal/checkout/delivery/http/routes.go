package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	cartDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

func RegisterRoutes(localized *gin.RouterGroup, service checkoutDomain.Service, carts cartDomain.Service, reservationTTL time.Duration, warehouseID uuid.UUID, secureCookies bool, sensitiveLimit gin.HandlerFunc) {
	if service == nil || carts == nil || warehouseID == uuid.Nil {
		return
	}
	handler := NewHandler(service, carts, reservationTTL, warehouseID, secureCookies)
	if sensitiveLimit == nil {
		localized.POST("/checkout/delivery-options", handler.QuoteDelivery)
		localized.POST("/checkout/payment", handler.StartPayment)
		return
	}
	checkout := localized.Group("/checkout")
	checkout.Use(sensitiveLimit)
	checkout.POST("/delivery-options", handler.QuoteDelivery)
	checkout.POST("/payment", handler.StartPayment)
}

// RegisterV1Routes adds the versioned transport contract without changing the
// legacy checkout endpoints.
func RegisterV1Routes(group *gin.RouterGroup, service checkoutDomain.Service, carts cartDomain.Service, reservationTTL time.Duration, warehouseID uuid.UUID, secureCookies bool, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || carts == nil || warehouseID == uuid.Nil || renderer == nil {
		return
	}
	handler := NewCheckoutV1Handler(NewHandler(service, carts, reservationTTL, warehouseID, secureCookies), renderer)
	group.POST("/delivery-options", handler.QuoteDelivery)
	group.POST("/payment", handler.StartPayment)
}

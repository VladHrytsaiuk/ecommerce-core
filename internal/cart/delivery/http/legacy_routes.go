//go:build legacy

package http

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

func RegisterCartRoutes(optionalAuthGrp *gin.RouterGroup, authGrp *gin.RouterGroup, s domain.CartService, l logger.Logger) {
	h := NewCartHandler(s, l)
	optionalAuthGrp.GET("/cart", h.GetCart)
	optionalAuthGrp.POST("/cart/items", h.AddToCart)
	optionalAuthGrp.PATCH("/cart/items/:variationId", h.UpdateCartItem)
	optionalAuthGrp.DELETE("/cart/items/:variationId", h.RemoveFromCart)
	optionalAuthGrp.POST("/cart/promo", h.ApplyPromoCode)
	optionalAuthGrp.DELETE("/cart/promo", h.RemovePromoCode)
	authGrp.POST("/cart/sync", h.SyncCart)
}

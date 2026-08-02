//go:build legacy
// +build legacy

package http

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

// RegisterCartRoutes реєструє маршрути модуля кошика.
//
// - GET, POST, PATCH, DELETE /cart — доступні як авторизованим (JWT), так і анонімним юзерам.
//   Анонімні юзери ідентифікуються через автоматичну куку guest_session.
// - POST /cart/sync — доступний тільки авторизованим юзерам (переносить дані з куки в профіль).

func RegisterCartRoutes(
	optionalAuthGrp *gin.RouterGroup,
	authGrp *gin.RouterGroup,
	s domain.CartService,
	l logger.Logger,
) {
	h := NewCartHandler(s, l)

	// Маршрути, доступні і анонімним, і авторизованим
	optionalAuthGrp.GET("/cart", h.GetCart)
	optionalAuthGrp.POST("/cart/items", h.AddToCart)
	optionalAuthGrp.PATCH("/cart/items/:variationId", h.UpdateCartItem)
	optionalAuthGrp.DELETE("/cart/items/:variationId", h.RemoveFromCart)
	optionalAuthGrp.POST("/cart/promo", h.ApplyPromoCode)
	optionalAuthGrp.DELETE("/cart/promo", h.RemovePromoCode)

	// Синхронізація — тільки для авторизованих
	authGrp.POST("/cart/sync", h.SyncCart)
}

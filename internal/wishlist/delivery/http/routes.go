//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"github.com/gin-gonic/gin"
)

// RegisterWishlistRoutes реєструє маршрути модуля вішліста.
//
// - GET, POST, DELETE /wishlist — доступні як авторизованим (JWT), так і анонімним юзерам.
//   Анонімні юзери ідентифікуються через автоматичну куку wishlist_session.
// - POST /wishlist/sync — доступний тільки авторизованим юзерам (переносить дані з куки в профіль).

func RegisterWishlistRoutes(
	optionalAuthGrp *gin.RouterGroup,
	authGrp *gin.RouterGroup,
	s domain.WishlistService,
	l logger.Logger,
) {
	h := NewWishlistHandler(s, l)

	// Маршрути, доступні і анонімним, і авторизованим
	optionalAuthGrp.GET("/wishlist", h.GetWishlist)
	optionalAuthGrp.POST("/wishlist/:variationId", h.AddToWishlist)
	optionalAuthGrp.DELETE("/wishlist/:variationId", h.RemoveFromWishlist)

	// Синхронізація — тільки для авторизованих
	authGrp.POST("/wishlist/sync", h.SyncWishlist)
}

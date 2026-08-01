package http

import (
	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// RegisterPromoRoutes реєструє маршрути для управління промокодами
func RegisterPromoRoutes(
	adminGrp *gin.RouterGroup,
	s domain.PromoService,
	l logger.Logger,
) {
	h := NewPromoHandler(s, l)

	// Admin API для промокодів
	promos := adminGrp.Group("/promos")
	{
		promos.POST("", h.CreatePromo)
		promos.GET("", h.ListPromos)
		promos.GET("/:id", h.GetPromo)
		promos.PUT("/:id", h.UpdatePromo)
		promos.DELETE("/:id", h.DeletePromo)
	}
}

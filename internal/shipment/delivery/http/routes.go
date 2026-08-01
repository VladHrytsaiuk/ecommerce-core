package http

import (
	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
)

// RegisterShipmentRoutes реєструє маршрути модуля доставки.
// Маршрути містять параметр :provider для підтримки кількох служб доставки.
func RegisterShipmentRoutes(rg *gin.RouterGroup, adminGrp *gin.RouterGroup, s domain.ShipmentService, l logger.Logger) {
	h := NewShipmentHandler(s, l)

	shipment := rg.Group("/shipment")
	{
		shipment.GET("/config", h.GetShippingConfig)

		providerGrp := shipment.Group("/:provider")
		{
			providerGrp.GET("/areas", h.GetAreas)
			providerGrp.GET("/cities", h.GetCities)
			providerGrp.GET("/warehouses", h.GetWarehouses)
		}
	}

	if adminGrp != nil {
		adminShipment := adminGrp.Group("/shipment")
		{
			adminShipment.POST("/rules", h.UpdateShippingRule)
		}
	}
}

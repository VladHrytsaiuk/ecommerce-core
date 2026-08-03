package http

import (
	"time"

	"github.com/gin-gonic/gin"

	checkoutDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
)

func RegisterRoutes(localized *gin.RouterGroup, service checkoutDomain.Service, reservationTTL time.Duration) {
	if service == nil {
		return
	}
	handler := NewHandler(service, reservationTTL)
	localized.POST("/checkout/payment", handler.StartPayment)
}

package http

import (
	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// RegisterWebhookRoutes реєструє маршрут вебхука для платежів.
// Ця група НЕ використовує AuthMiddleware — валідація через підпис LiqPay.
func RegisterWebhookRoutes(
	webhookGrp *gin.RouterGroup,
	s domain.PaymentService,
	l logger.Logger,
) {
	h := NewPaymentHandler(s, l)

	webhookGrp.POST("/liqpay", h.HandleLiqPayWebhook)
	webhookGrp.POST("/mock-payment", h.HandleMockPayment)
}

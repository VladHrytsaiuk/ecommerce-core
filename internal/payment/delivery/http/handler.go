package http

import (
	"errors"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// PaymentHandler обробляє вебхуки платежів
type PaymentHandler struct {
	service domain.PaymentService
	l       logger.Logger
}

// NewPaymentHandler створює новий інстанс хендлера
func NewPaymentHandler(s domain.PaymentService, l logger.Logger) *PaymentHandler {
	return &PaymentHandler{service: s, l: l}
}

// HandleLiqPayWebhook godoc
// @Summary      LiqPay payment webhook
// @Description  Receives payment callback from LiqPay. No authentication — validated by signature.
// @Tags         Webhooks
// @Accept       application/x-www-form-urlencoded
// @Produce      json
// @Param        data formData string true "Base64-encoded JSON payload"
// @Param        signature formData string true "LiqPay signature"
// @Success      200  {object} map[string]string
// @Failure      400  {object} map[string]string
// @Failure      403  {object} map[string]string
// @Router       /api/webhooks/liqpay [post]
func (h *PaymentHandler) HandleLiqPayWebhook(c *gin.Context) {
	var callback domain.LiqPayCallback

	// LiqPay надсилає дані як application/x-www-form-urlencoded
	if err := c.ShouldBind(&callback); err != nil {
		h.l.Warnw("LiqPay webhook: invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "invalid request"})
		return
	}

	err := h.service.ProcessWebhook(c.Request.Context(), callback.Data, callback.Signature)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidSignature) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "invalid signature"})
			return
		}

		h.l.Errorw("LiqPay webhook processing error", "error", err)
		// LiqPay вимагає 200 OK навіть при помилках, інакше буде повторювати запити
		c.JSON(http.StatusOK, gin.H{"status": "error", "message": "processing failed"})
		return
	}

	// LiqPay вимагає 200 OK
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type MockPaymentRequest struct {
	OrderID string `json:"order_id" binding:"required,uuid"`
}

// HandleMockPayment godoc
// @Summary      Mock payment (Development only)
// @Description  Simulates a successful payment. Only works if APP_ENV is development or local.
// @Tags         Webhooks
// @Accept       json
// @Produce      json
// @Param        request body MockPaymentRequest true "Order ID to simulate payment for"
// @Success      200  {object} map[string]string
// @Failure      400  {object} map[string]string
// @Failure      403  {object} map[string]string
// @Router       /api/webhooks/mock-payment [post]
func (h *PaymentHandler) HandleMockPayment(c *gin.Context) {
	appEnv := os.Getenv("APP_ENV")
	if appEnv != "development" && appEnv != "local" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Mock payment is only available in development mode"})
		return
	}

	var req MockPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "invalid request"})
		return
	}

	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "invalid order_id"})
		return
	}

	if err := h.service.SimulatePayment(c.Request.Context(), orderID); err != nil {
		h.l.Errorw("Mock payment simulation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "simulation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "payment simulated successfully"})
}

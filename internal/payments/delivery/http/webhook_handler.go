// Package http maps provider callback HTTP requests to the provider-neutral
// payment webhook application service.
package http

import (
	"errors"
	"io"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"

	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
)

const maxWebhookBodySize = 1 << 20

type WebhookHandler struct{ service *paymentsApp.WebhookService }

func NewWebhookHandler(service *paymentsApp.WebhookService) *WebhookHandler {
	return &WebhookHandler{service: service}
}

func (h *WebhookHandler) Handle(c *gin.Context) {
	// Keep the original byte sequence intact: gateway signature schemes (notably
	// Monobank's ECDSA X-Sign) sign the raw HTTP body, not a re-marshaled JSON
	// representation. MaxBytesReader also reports an oversized request instead
	// of silently truncating it as io.LimitReader would.
	c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBodySize)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var tooLarge *stdhttp.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(stdhttp.StatusRequestEntityTooLarge, gin.H{"error": "webhook payload is too large"})
			return
		}
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}
	if err := h.service.Handle(c.Request.Context(), c.Param("provider"), paymentsDomain.WebhookRequest{RequestID: c.GetHeader("X-Request-ID"), Headers: flattenHeaders(c.Request.Header), Payload: body}); err != nil {
		switch {
		case errors.Is(err, paymentsDomain.ErrInvalidWebhookSignature):
			c.JSON(stdhttp.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		case errors.Is(err, paymentsDomain.ErrGatewayNotEnabled):
			c.JSON(stdhttp.StatusNotFound, gin.H{"error": "payment gateway is not enabled"})
		default:
			c.JSON(stdhttp.StatusInternalServerError, gin.H{"error": "payment webhook processing failed"})
		}
		return
	}
	c.Status(stdhttp.StatusNoContent)
}

func RegisterWebhookRoutes(api *gin.RouterGroup, service *paymentsApp.WebhookService) {
	if service == nil {
		return
	}
	handler := NewWebhookHandler(service)
	api.POST("/webhooks/payments/:provider", handler.Handle)
}

func flattenHeaders(headers stdhttp.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

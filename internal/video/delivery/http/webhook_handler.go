package http

import (
	"errors"
	"io"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

const maxWebhookBodyBytes = 1 << 20

func RegisterWebhookRoutes(group *gin.RouterGroup, service *videoApp.WebhookService, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	group.POST("/webhooks/cloudflare/stream", receiveCloudflareWebhook(service, renderer))
}

// receiveCloudflareWebhook godoc
// @Summary Receive a Cloudflare Stream encoding webhook
// @Description Verifies Cloudflare's raw-body HMAC signature before changing a video asset.
// @Tags Video webhooks
// @Accept json
// @Success 204
// @Failure 400,401,413,500 {object} apiresponse.ProblemDetails
// @Router /api/v1/webhooks/cloudflare/stream [post]
func receiveCloudflareWebhook(service *videoApp.WebhookService, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBodyBytes)
		rawPayload, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var tooLarge *stdhttp.MaxBytesError
			if errors.As(err, &tooLarge) {
				renderer.Abort(c, apiresponse.PayloadTooLarge(err))
			} else {
				renderer.Abort(c, apiresponse.InvalidPayload(err))
			}
			return
		}
		// Current Cloudflare Stream documentation names this Webhook-Signature.
		// Cf-Webhook-Signature remains accepted for existing gateway deployments.
		signature := c.GetHeader("Cf-Webhook-Signature")
		if signature == "" {
			signature = c.GetHeader("Webhook-Signature")
		}
		if err := service.Handle(c.Request.Context(), rawPayload, signature); err != nil {
			switch {
			case errors.Is(err, video.ErrInvalidWebhookSignature):
				renderer.Abort(c, apiresponse.Unauthenticated(err))
			case errors.Is(err, video.ErrInvalidWebhookPayload):
				renderer.Abort(c, apiresponse.InvalidPayload(err))
			default:
				renderer.Abort(c, err)
			}
			return
		}
		c.Status(stdhttp.StatusNoContent)
	}
}

func RegisterStorefrontRoutes(group *gin.RouterGroup, reader video.StorefrontReader, renderer *apiresponse.ErrorRenderer) {
	if group == nil || reader == nil || renderer == nil {
		return
	}
	group.GET("/:id/videos", listProductVideos(reader, renderer))
}

// listProductVideos godoc
// @Summary List ready public videos for a product
// @Tags Catalog v1
// @Produce json
// @Param id path string true "Product ID"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,500 {object} apiresponse.ProblemDetails
// @Router /api/v1/catalog/products/{id}/videos [get]
func listProductVideos(reader video.StorefrontReader, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		productID, err := uuid.Parse(c.Param("id"))
		if err != nil || productID == uuid.Nil {
			renderer.Abort(c, apiresponse.InvalidPayload(errors.New("invalid product ID")))
			return
		}
		videos, err := reader.ListReadyProductVideos(c.Request.Context(), productID)
		if err != nil {
			renderer.Abort(c, err)
			return
		}
		response := make([]gin.H, 0, len(videos))
		for _, item := range videos {
			// Provider and external IDs remain strictly server-side. Public clients
			// need our product-video ID and safe display metadata only; a future
			// signed playback URL belongs here, not a Cloudflare identifier.
			response = append(response, gin.H{"id": item.ID, "role": item.Role, "position": item.Position, "duration_seconds": item.Asset.DurationSeconds, "poster_url": item.Asset.PosterURL})
		}
		apiresponse.Success(c, stdhttp.StatusOK, response)
	}
}

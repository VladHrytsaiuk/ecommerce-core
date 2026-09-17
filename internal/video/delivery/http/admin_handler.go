// Package http exposes the permission-gated Video admin transport.
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
)

func RegisterV1Routes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, service *videoApp.DirectUploadService, renderer *apiresponse.ErrorRenderer) {
	if group == nil || service == nil || renderer == nil {
		return
	}
	group.POST("/videos/upload-url", shared.RequirePermissionV1(authorizer, videoApp.PermissionVideoWrite, renderer), createDirectUpload(service, renderer))
}

type createDirectUploadRequest struct {
	MaxDurationSeconds int `json:"max_duration_seconds"`
}

// createDirectUpload godoc
// @Summary Create a Cloudflare Stream direct-upload URL
// @Description Creates a server-tracked video asset and returns a short-lived TUS upload URL.
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param request body createDirectUploadRequest false "Direct-upload constraints"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,500,503 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/videos/upload-url [post]
func createDirectUpload(service *videoApp.DirectUploadService, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request createDirectUploadRequest
		if c.Request.ContentLength > 0 && c.ShouldBindJSON(&request) != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(errors.New("invalid video direct-upload payload")))
			return
		}
		asset, instruction, err := service.CreateDirectUpload(c.Request.Context(), videoApp.CreateDirectUploadCommand{MaxDurationSeconds: request.MaxDurationSeconds})
		if err != nil {
			renderer.Abort(c, apiresponse.Unavailable(err))
			return
		}
		apiresponse.Success(c, http.StatusCreated, gin.H{"video_id": asset.ID, "external_id": instruction.ExternalID, "upload_url": instruction.UploadURL})
	}
}

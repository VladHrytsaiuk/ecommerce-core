package http

import (
	"errors"
	stdhttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	videoApp "github.com/VladHrytsaiuk/ecommerce-core/internal/video/application"
	video "github.com/VladHrytsaiuk/ecommerce-core/internal/video/domain"
)

// RegisterV1PlacementRoutes exposes product video administration. Every
// mutation goes through the facade, which writes its audit event in the same
// transaction; the router deliberately exposes no unaudited admin route.
func RegisterV1PlacementRoutes(group *gin.RouterGroup, authorizer adminDomain.Authorizer, facade *videoApp.PlacementAdminFacade, renderer *apiresponse.ErrorRenderer) {
	if group == nil || authorizer == nil || facade == nil || renderer == nil {
		return
	}
	guard := shared.RequirePermissionV1(authorizer, videoApp.PermissionVideoWrite, renderer)
	products := group.Group("/products/:productID/videos")
	products.GET("", guard, listPlacements(facade, renderer))
	products.POST("", guard, attachPlacement(facade, renderer))
	products.PATCH("/:placementID", guard, updatePlacement(facade, renderer))
	products.DELETE("/:placementID", guard, detachPlacement(facade, renderer))
}

type attachPlacementRequest struct {
	VideoAssetID uuid.UUID `json:"video_asset_id" binding:"required"`
	Role         string    `json:"role" binding:"required"`
	Position     int       `json:"position"`
	IsVisible    *bool     `json:"is_visible"`
}

type updatePlacementRequest struct {
	Position  int   `json:"position"`
	IsVisible *bool `json:"is_visible"`
}

// listPlacements godoc
// @Summary List every video placed on a product (v1 admin)
// @Description Includes hidden placements and assets that are not ready, which the storefront route deliberately omits.
// @Tags Admin v1
// @Produce json
// @Param productID path string true "Product UUID"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/products/{productID}/videos [get]
func listPlacements(facade *videoApp.PlacementAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID, productID, ok := placementScope(c, renderer)
		if !ok {
			return
		}
		placements, err := facade.List(c.Request.Context(), actorID, productID)
		if err != nil {
			renderer.Abort(c, placementError(err))
			return
		}
		response := make([]gin.H, 0, len(placements))
		for _, placement := range placements {
			response = append(response, adminPlacementResponse(placement))
		}
		apiresponse.Success(c, stdhttp.StatusOK, response)
	}
}

// attachPlacement godoc
// @Summary Place a ready video on a product (v1 admin)
// @Description Refuses an asset that has not finished encoding, so a placement can never point at a draft or failed upload.
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param productID path string true "Product UUID"
// @Param payload body attachPlacementRequest true "Placement"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,409,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/products/{productID}/videos [post]
func attachPlacement(facade *videoApp.PlacementAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID, productID, ok := placementScope(c, renderer)
		if !ok {
			return
		}
		var request attachPlacementRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(errors.New("invalid product video placement payload")))
			return
		}
		placed, err := facade.Attach(c.Request.Context(), videoApp.AttachPlacementCommand{
			ActorUserID: actorID, EventKey: idempotencyKey(c), ProductID: productID,
			AssetID: request.VideoAssetID, Role: strings.ToLower(strings.TrimSpace(request.Role)),
			Position: request.Position, IsVisible: visibility(request.IsVisible),
		})
		if err != nil {
			renderer.Abort(c, placementError(err))
			return
		}
		apiresponse.Success(c, stdhttp.StatusCreated, adminPlacementResponse(placed))
	}
}

// updatePlacement godoc
// @Summary Reorder or hide a product video placement (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param productID path string true "Product UUID"
// @Param placementID path string true "Placement UUID"
// @Param payload body updatePlacementRequest true "New position and visibility"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,409,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/products/{productID}/videos/{placementID} [patch]
func updatePlacement(facade *videoApp.PlacementAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID, productID, ok := placementScope(c, renderer)
		if !ok {
			return
		}
		placementID, ok := placementIdentifier(c, renderer)
		if !ok {
			return
		}
		var request updatePlacementRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(errors.New("invalid product video placement payload")))
			return
		}
		updated, err := facade.Update(c.Request.Context(), videoApp.UpdatePlacementCommand{
			ActorUserID: actorID, EventKey: idempotencyKey(c), ProductID: productID,
			PlacementID: placementID, Position: request.Position, IsVisible: visibility(request.IsVisible),
		})
		if err != nil {
			renderer.Abort(c, placementError(err))
			return
		}
		apiresponse.Success(c, stdhttp.StatusOK, adminPlacementResponse(updated))
	}
}

// detachPlacement godoc
// @Summary Remove a video from a product (v1 admin)
// @Description Removes the placement only. The underlying asset is retained and reclaimed separately by the orphan reconciler.
// @Tags Admin v1
// @Produce json
// @Param productID path string true "Product UUID"
// @Param placementID path string true "Placement UUID"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 204 "No Content"
// @Failure 400,401,403,404 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/products/{productID}/videos/{placementID} [delete]
func detachPlacement(facade *videoApp.PlacementAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID, productID, ok := placementScope(c, renderer)
		if !ok {
			return
		}
		placementID, ok := placementIdentifier(c, renderer)
		if !ok {
			return
		}
		if err := facade.Detach(c.Request.Context(), videoApp.DetachPlacementCommand{
			ActorUserID: actorID, EventKey: idempotencyKey(c), ProductID: productID, PlacementID: placementID,
		}); err != nil {
			renderer.Abort(c, placementError(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

// adminPlacementResponse exposes provider identifiers, unlike the storefront
// projection: an administrator needs them to reconcile against Cloudflare.
func adminPlacementResponse(placement video.ProductVideo) gin.H {
	return gin.H{
		"id": placement.ID, "product_id": placement.ProductID, "video_asset_id": placement.AssetID,
		"role": placement.Role, "position": placement.Position, "is_visible": placement.IsVisible,
		"status": placement.Asset.Status, "external_id": placement.Asset.ExternalID,
		"duration_seconds": placement.Asset.DurationSeconds, "poster_url": placement.Asset.PosterURL,
	}
}

func placementScope(c *gin.Context, renderer *apiresponse.ErrorRenderer) (uuid.UUID, uuid.UUID, bool) {
	actorID, ok := shared.AuthenticatedUserID(c)
	if !ok {
		renderer.Abort(c, apiresponse.Unauthenticated(errors.New("authenticated subject missing")))
		return uuid.Nil, uuid.Nil, false
	}
	productID, err := uuid.Parse(c.Param("productID"))
	if err != nil || productID == uuid.Nil {
		renderer.Abort(c, apiresponse.InvalidPayload(errors.New("invalid product ID")))
		return uuid.Nil, uuid.Nil, false
	}
	return actorID, productID, true
}

func placementIdentifier(c *gin.Context, renderer *apiresponse.ErrorRenderer) (uuid.UUID, bool) {
	placementID, err := uuid.Parse(c.Param("placementID"))
	if err != nil || placementID == uuid.Nil {
		renderer.Abort(c, apiresponse.InvalidPayload(errors.New("invalid placement ID")))
		return uuid.Nil, false
	}
	return placementID, true
}

// visibility defaults a placement to visible. Attaching a video and finding it
// absent from the storefront is the more surprising outcome.
func visibility(requested *bool) bool { return requested == nil || *requested }

// idempotencyKey reuses the caller's key as the audit event identity, so a
// retried mutation is deduplicated by the Outbox rather than audited twice.
func idempotencyKey(c *gin.Context) uuid.UUID {
	header := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if header == "" {
		return uuid.Nil
	}
	key, err := uuid.Parse(header)
	if err != nil {
		return uuid.Nil
	}
	return key
}

// placementError maps domain outcomes onto transport semantics. Without it an
// occupied slot or an unfinished encoding would surface as a 500.
func placementError(err error) error {
	switch {
	case errors.Is(err, video.ErrPlacementNotFound), errors.Is(err, video.ErrAssetNotFound):
		return apiresponse.NotFound(err, "product video placement was not found")
	case errors.Is(err, video.ErrInvalidPlacement):
		return apiresponse.InvalidPayload(err)
	case errors.Is(err, video.ErrAssetNotPlayable):
		return apiresponse.ValidationFailed(err)
	case errors.Is(err, video.ErrPositionTaken), errors.Is(err, video.ErrPlacementDuplicate):
		return &apiresponse.PublicError{Status: stdhttp.StatusConflict, Code: "PLACEMENT_CONFLICT", Detail: err.Error(), Cause: err}
	default:
		return err
	}
}

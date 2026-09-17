package http

import (
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	reviewsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
)

// RegisterV1ContentRoutes exposes badges, review moderation and SEO metadata.
//
// These were deliberately left unregistered until each mutation could append
// an audit event in the same transaction as the change; every route here
// reaches the facade that does so. Each module's routes appear only when that
// module is enabled.
func RegisterV1ContentRoutes(g *gin.RouterGroup, authorizer adminDomain.Authorizer, facade *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) {
	if g == nil || facade == nil || renderer == nil {
		return
	}
	if facade.HasBadges() {
		guard := RequirePermissionV1(authorizer, adminApp.PermissionBadgesWrite, renderer)
		g.POST("/badges", guard, createBadgeV1(facade, renderer))
		g.PUT("/badges/:badge_id", guard, updateBadgeV1(facade, renderer))
		g.DELETE("/badges/:badge_id", guard, deleteBadgeV1(facade, renderer))
		g.PUT("/badges/:badge_id/products/:product_id", guard, badgeAssignmentV1(facade, renderer, true))
		g.DELETE("/badges/:badge_id/products/:product_id", guard, badgeAssignmentV1(facade, renderer, false))
	}
	if facade.HasReviews() {
		guard := RequirePermissionV1(authorizer, adminApp.PermissionReviewsWrite, renderer)
		g.PATCH("/reviews/:review_id/status", guard, moderateReviewV1(facade, renderer))
		g.DELETE("/reviews/:review_id", guard, deleteReviewV1(facade, renderer))
	}
	if facade.HasSEO() {
		guard := RequirePermissionV1(authorizer, adminApp.PermissionSEOWrite, renderer)
		g.PUT("/seo", guard, upsertSEOV1(facade, renderer))
		g.DELETE("/seo/:resource_type/:resource_id/:locale", guard, deleteSEOV1(facade, renderer))
	}
}

type badgeRequest struct {
	Slug         string             `json:"slug"`
	Color        string             `json:"color"`
	Translations []badgeTranslation `json:"translations"`
}

type badgeTranslation struct {
	Locale string `json:"locale"`
	Name   string `json:"name"`
}

func (r badgeRequest) translations() []badgesDomain.Translation {
	translations := make([]badgesDomain.Translation, 0, len(r.Translations))
	for _, translation := range r.Translations {
		translations = append(translations, badgesDomain.Translation{Locale: translation.Locale, Name: translation.Name})
	}
	return translations
}

type reviewStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type seoRequest struct {
	ResourceType string `json:"resource_type" binding:"required"`
	ResourceID   string `json:"resource_id" binding:"required"`
	Locale       string `json:"locale" binding:"required"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Keywords     string `json:"keywords"`
	OGImageRef   string `json:"og_image_ref"`
}

// createBadgeV1 godoc
// @Summary Create a catalog badge (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param payload body badgeRequest true "Badge"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/badges [post]
func createBadgeV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		var request badgeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		badge, err := f.CreateBadge(c.Request.Context(), cmd, badgesDomain.CreateCommand{
			Slug: request.Slug, Color: request.Color, Translations: request.translations(),
		})
		if err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.Success(c, http.StatusCreated, badge)
	}
}

// updateBadgeV1 godoc
// @Summary Update a catalog badge (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param badge_id path string true "Badge UUID"
// @Param payload body badgeRequest true "Badge"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/badges/{badge_id} [put]
func updateBadgeV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		badgeID, ok := contentID(c, renderer, "badge_id")
		if !ok {
			return
		}
		var request badgeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		badge, err := f.UpdateBadge(c.Request.Context(), cmd, badgeID, badgesDomain.UpdateCommand{
			Slug: request.Slug, Color: request.Color, Translations: request.translations(),
		})
		if err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, badge)
	}
}

// deleteBadgeV1 godoc
// @Summary Delete a catalog badge (v1 admin)
// @Tags Admin v1
// @Produce json
// @Param badge_id path string true "Badge UUID"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 204 "No Content"
// @Failure 400,401,403,404 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/badges/{badge_id} [delete]
func deleteBadgeV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		badgeID, ok := contentID(c, renderer, "badge_id")
		if !ok {
			return
		}
		if err := f.DeleteBadge(c.Request.Context(), cmd, badgeID); err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

// badgeAssignmentV1 godoc
// @Summary Attach or detach a badge on a product (v1 admin)
// @Tags Admin v1
// @Produce json
// @Param badge_id path string true "Badge UUID"
// @Param product_id path string true "Product UUID"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 204 "No Content"
// @Failure 400,401,403,404 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/badges/{badge_id}/products/{product_id} [put]
func badgeAssignmentV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer, assign bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		badgeID, ok := contentID(c, renderer, "badge_id")
		if !ok {
			return
		}
		productID, ok := contentID(c, renderer, "product_id")
		if !ok {
			return
		}
		var err error
		if assign {
			err = f.AssignBadge(c.Request.Context(), cmd, badgeID, productID)
		} else {
			err = f.RemoveBadge(c.Request.Context(), cmd, badgeID, productID)
		}
		if err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

// moderateReviewV1 godoc
// @Summary Approve or reject a product review (v1 admin)
// @Description The resulting status is recorded in the audit trail; moderation is the most contested admin action there is.
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param review_id path string true "Review UUID"
// @Param payload body reviewStatusRequest true "New status"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,404,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reviews/{review_id}/status [patch]
func moderateReviewV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		reviewID, ok := contentID(c, renderer, "review_id")
		if !ok {
			return
		}
		var request reviewStatusRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		review, err := f.ModerateReview(c.Request.Context(), cmd, reviewID, reviewsDomain.Status(request.Status))
		if err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, review)
	}
}

// deleteReviewV1 godoc
// @Summary Delete a product review (v1 admin)
// @Tags Admin v1
// @Produce json
// @Param review_id path string true "Review UUID"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 204 "No Content"
// @Failure 400,401,403,404 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/reviews/{review_id} [delete]
func deleteReviewV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		reviewID, ok := contentID(c, renderer, "review_id")
		if !ok {
			return
		}
		if err := f.DeleteReview(c.Request.Context(), cmd, reviewID); err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

// upsertSEOV1 godoc
// @Summary Replace SEO metadata for one resource and locale (v1 admin)
// @Tags Admin v1
// @Accept json
// @Produce json
// @Param payload body seoRequest true "Metadata"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400,401,403,422 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/seo [put]
func upsertSEOV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		var request seoRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			renderer.Abort(c, apiresponse.InvalidPayload(err))
			return
		}
		resourceID, err := uuid.Parse(request.ResourceID)
		if err != nil || resourceID == uuid.Nil {
			renderer.Abort(c, apiresponse.InvalidPayload(stderrors.New("invalid SEO resource ID")))
			return
		}
		metadata, err := f.UpsertSEO(c.Request.Context(), cmd, seoDomain.UpsertCommand{
			ResourceType: request.ResourceType, ResourceID: resourceID, Locale: request.Locale,
			Title: request.Title, Description: request.Description,
			Keywords: request.Keywords, OGImageRef: request.OGImageRef,
		})
		if err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.Success(c, http.StatusOK, metadata)
	}
}

// deleteSEOV1 godoc
// @Summary Delete SEO metadata for one resource and locale (v1 admin)
// @Tags Admin v1
// @Produce json
// @Param resource_type path string true "Resource type"
// @Param resource_id path string true "Resource UUID"
// @Param locale path string true "Locale code"
// @Param Idempotency-Key header string false "Reuses the audit identity of a retried request"
// @Success 204 "No Content"
// @Failure 400,401,403,404 {object} apiresponse.ProblemDetails
// @Router /api/v1/admin/seo/{resource_type}/{resource_id}/{locale} [delete]
func deleteSEOV1(f *adminApp.ContentAdminFacade, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		cmd, ok := contentCommand(c, renderer)
		if !ok {
			return
		}
		resourceID, ok := contentID(c, renderer, "resource_id")
		if !ok {
			return
		}
		if err := f.DeleteSEO(c.Request.Context(), cmd, c.Param("resource_type"), resourceID, c.Param("locale")); err != nil {
			renderer.Abort(c, contentError(err))
			return
		}
		apiresponse.NoContent(c)
	}
}

func contentCommand(c *gin.Context, renderer *apiresponse.ErrorRenderer) (adminApp.CatalogCommand, bool) {
	actor, ok := shared.AuthenticatedUserID(c)
	if !ok {
		renderer.Abort(c, apiresponse.Unauthenticated(stderrors.New("authenticated subject missing")))
		return adminApp.CatalogCommand{}, false
	}
	return adminApp.CatalogCommand{ActorUserID: actor, EventKey: idempotency(c), IPAddress: c.ClientIP()}, true
}

func contentID(c *gin.Context, renderer *apiresponse.ErrorRenderer, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil || id == uuid.Nil {
		renderer.Abort(c, apiresponse.InvalidPayload(stderrors.New("invalid "+param)))
		return uuid.Nil, false
	}
	return id, true
}

// contentError maps module outcomes onto transport semantics, so a missing
// badge is a 404 rather than the 500 an unmapped error would produce.
func contentError(err error) error {
	switch {
	case stderrors.Is(err, badgesDomain.ErrNotFound), stderrors.Is(err, seoDomain.ErrNotFound), stderrors.Is(err, reviewsDomain.ErrNotFound):
		return apiresponse.NotFound(err, "The requested record was not found.")
	case stderrors.Is(err, badgesDomain.ErrInvalidBadge), stderrors.Is(err, seoDomain.ErrInvalidMetadata), stderrors.Is(err, reviewsDomain.ErrInvalidReview):
		return apiresponse.ValidationFailed(err)
	default:
		return err
	}
}

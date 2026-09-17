// Package http maps the public Reviews API to the module service.
//
// Moderation is deliberately absent. An earlier version of this file also
// registered PATCH and DELETE under an admin group with no authorization and
// no audit record, which is the forensic bypass router.go refuses to expose;
// those actions belong to ContentAdminFacade, which writes the change and its
// audit entry in one transaction.
package http

import (
	"errors"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

// RegisterV1Routes exposes reading approved reviews and submitting one.
//
// Without these the module was unusable: a store could enable reviews, seed
// the permission and moderate through the admin API, while no customer could
// ever write one or see one.
func RegisterV1Routes(v1 *gin.RouterGroup, service domain.Service, authMiddleware gin.HandlerFunc, renderer *apiresponse.ErrorRenderer) {
	if v1 == nil || service == nil {
		return
	}
	handler := &Handler{service: service}
	group := v1.Group("/catalog/products/:product_id/reviews")
	group.GET("", handler.listApproved(renderer))
	if authMiddleware != nil {
		// Submission is authenticated: a review is attributed to a customer,
		// and the repository's uniqueness rule is per customer and product.
		group.POST("", authMiddleware, handler.create(renderer))
	}
}

type Handler struct{ service domain.Service }

type createRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

type reviewResponse struct {
	ID        string        `json:"id"`
	ProductID string        `json:"product_id"`
	Rating    int           `json:"rating"`
	Comment   string        `json:"comment"`
	Status    domain.Status `json:"status"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
}

// create godoc
// @Summary Submit a product review
// @Description Requires authentication; the review is attributed to the caller and starts pending moderation. A customer may review a product once.
// @Tags Catalog v1
// @Accept json
// @Produce json
// @Param product_id path string true "Product UUID"
// @Param payload body createRequest true "Rating and comment"
// @Success 201 {object} apiresponse.SuccessResponse
// @Failure 400,401,409,422 {object} apiresponse.ProblemDetails
// @Security bearerAuth
// @Router /api/v1/catalog/products/{product_id}/reviews [post]
func (handler *Handler) create(renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(context *gin.Context) {
		userID, ok := middleware.AuthenticatedUserID(context)
		if !ok {
			renderer.Abort(context, apiresponse.Unauthenticated(nil))
			return
		}
		productID, ok := parseProductID(context, renderer)
		if !ok {
			return
		}
		var request createRequest
		if err := context.ShouldBindJSON(&request); err != nil {
			renderer.Abort(context, apiresponse.InvalidPayload(err))
			return
		}
		review, err := handler.service.Create(context.Request.Context(), domain.CreateCommand{
			ProductID: productID, UserID: userID, Rating: request.Rating, Comment: request.Comment,
		})
		if err != nil {
			abort(renderer, context, err)
			return
		}
		// The response carries the pending status rather than implying the
		// review is already visible to other customers.
		apiresponse.Success(context, stdhttp.StatusCreated, toResponse(review))
	}
}

// listApproved godoc
// @Summary List approved reviews for a product
// @Description Returns only reviews an administrator has approved. Pending and rejected reviews are never exposed, including to their own author.
// @Tags Catalog v1
// @Produce json
// @Param product_id path string true "Product UUID"
// @Success 200 {object} apiresponse.SuccessResponse
// @Failure 400 {object} apiresponse.ProblemDetails
// @Router /api/v1/catalog/products/{product_id}/reviews [get]
func (handler *Handler) listApproved(renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(context *gin.Context) {
		productID, ok := parseProductID(context, renderer)
		if !ok {
			return
		}
		reviews, err := handler.service.ListApproved(context.Request.Context(), productID)
		if err != nil {
			abort(renderer, context, err)
			return
		}
		response := make([]reviewResponse, 0, len(reviews))
		for index := range reviews {
			response = append(response, toResponse(&reviews[index]))
		}
		apiresponse.Success(context, stdhttp.StatusOK, gin.H{"reviews": response})
	}
}

func parseProductID(context *gin.Context, renderer *apiresponse.ErrorRenderer) (uuid.UUID, bool) {
	id, err := uuid.Parse(context.Param("product_id"))
	if err != nil || id == uuid.Nil {
		renderer.Abort(context, apiresponse.InvalidPayload(err, apiresponse.InvalidParam{Field: "product_id", Code: "INVALID_UUID"}))
		return uuid.Nil, false
	}
	return id, true
}

func abort(renderer *apiresponse.ErrorRenderer, context *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidReview):
		renderer.Abort(context, apiresponse.ValidationFailed(err))
	case errors.Is(err, domain.ErrAlreadyExists):
		renderer.Abort(context, &apiresponse.PublicError{
			Cause: err, Status: stdhttp.StatusConflict, Code: apiresponse.CodeConflict,
			Title: "Review already exists", Detail: "You have already reviewed this product.",
		})
	case errors.Is(err, domain.ErrNotFound):
		renderer.Abort(context, apiresponse.NotFound(err, "Review not found."))
	default:
		renderer.Abort(context, err)
	}
}

func toResponse(review *domain.Review) reviewResponse {
	return reviewResponse{
		ID: review.ID.String(), ProductID: review.ProductID.String(),
		Rating: review.Rating, Comment: review.Comment, Status: review.Status,
		CreatedAt: review.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: review.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

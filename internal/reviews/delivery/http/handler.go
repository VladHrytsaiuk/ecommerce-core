// Package http maps the public and admin Reviews API to the module service.
package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/reviews/domain"
)

type Handler struct{ service domain.Service }

func NewHandler(service domain.Service) *Handler { return &Handler{service: service} }

type createRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

type statusRequest struct {
	Status domain.Status `json:"status"`
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

func (handler *Handler) Create(context *gin.Context) {
	userID, ok := middleware.AuthenticatedUserID(context)
	if !ok {
		context.AbortWithStatusJSON(stdhttp.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	productID, ok := parseID(context, "product_id")
	if !ok {
		return
	}
	var request createRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid review request"})
		return
	}
	review, err := handler.service.Create(context.Request.Context(), domain.CreateCommand{ProductID: productID, UserID: userID, Rating: request.Rating, Comment: request.Comment})
	if err != nil {
		handleError(context, err)
		return
	}
	context.JSON(stdhttp.StatusCreated, toResponse(review))
}

func (handler *Handler) ListApproved(context *gin.Context) {
	productID, ok := parseID(context, "product_id")
	if !ok {
		return
	}
	reviews, err := handler.service.ListApproved(context.Request.Context(), productID)
	if err != nil {
		handleError(context, err)
		return
	}
	response := make([]reviewResponse, 0, len(reviews))
	for index := range reviews {
		response = append(response, toResponse(&reviews[index]))
	}
	context.JSON(stdhttp.StatusOK, gin.H{"reviews": response})
}

func (handler *Handler) SetStatus(context *gin.Context) {
	reviewID, ok := parseID(context, "review_id")
	if !ok {
		return
	}
	var request statusRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid review status"})
		return
	}
	review, err := handler.service.SetStatus(context.Request.Context(), reviewID, request.Status)
	if err != nil {
		handleError(context, err)
		return
	}
	context.JSON(stdhttp.StatusOK, toResponse(review))
}

func (handler *Handler) Delete(context *gin.Context) {
	reviewID, ok := parseID(context, "review_id")
	if !ok {
		return
	}
	if err := handler.service.Delete(context.Request.Context(), reviewID); err != nil {
		handleError(context, err)
		return
	}
	context.Status(stdhttp.StatusNoContent)
}

func parseID(context *gin.Context, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(context.Param(key))
	if err != nil || id == uuid.Nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid identifier"})
		return uuid.Nil, false
	}
	return id, true
}

func handleError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidReview):
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid review"})
	case errors.Is(err, domain.ErrAlreadyExists):
		context.AbortWithStatusJSON(stdhttp.StatusConflict, gin.H{"error": "review already exists for product"})
	case errors.Is(err, domain.ErrNotFound):
		context.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "review not found"})
	default:
		context.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "reviews unavailable"})
	}
}

func toResponse(review *domain.Review) reviewResponse {
	return reviewResponse{ID: review.ID.String(), ProductID: review.ProductID.String(), Rating: review.Rating, Comment: review.Comment, Status: review.Status, CreatedAt: review.CreatedAt.UTC().Format(stdhttp.TimeFormat), UpdatedAt: review.UpdatedAt.UTC().Format(stdhttp.TimeFormat)}
}

func RegisterRoutes(api, admin *gin.RouterGroup, service domain.Service, authMiddleware gin.HandlerFunc) {
	if service == nil {
		return
	}
	handler := NewHandler(service)
	public := api.Group("/reviews")
	public.GET("/:product_id", handler.ListApproved)
	if authMiddleware != nil {
		public.POST("/:product_id", authMiddleware, handler.Create)
	}
	admin.PATCH("/reviews/:review_id/status", handler.SetStatus)
	admin.DELETE("/reviews/:review_id", handler.Delete)
}

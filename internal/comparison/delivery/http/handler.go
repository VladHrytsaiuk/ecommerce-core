// Package http exposes the optional Comparison module through provider-neutral
// HTTP handlers.
package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	comparisonDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
)

type itemResponse struct {
	ID               string `json:"id"`
	ProductVariantID string `json:"product_variant_id"`
	CreatedAt        string `json:"created_at"`
}

type listResponse struct {
	ID         string         `json:"id"`
	CategoryID string         `json:"category_id"`
	Items      []itemResponse `json:"items"`
}

type Handler struct {
	service       comparisonDomain.Service
	secureCookies bool
}

func NewHandler(service comparisonDomain.Service, secureCookies bool) *Handler {
	return &Handler{service: service, secureCookies: secureCookies}
}

func (handler *Handler) List(context *gin.Context) {
	owner, err := handler.owner(context)
	if err != nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid comparison owner"})
		return
	}
	comparison, err := handler.service.List(context.Request.Context(), owner)
	if err != nil {
		handleError(context, err)
		return
	}
	context.JSON(stdhttp.StatusOK, gin.H{"lists": listsResponse(comparison.Lists)})
}

func listsResponse(lists []comparisonDomain.List) []listResponse {
	response := make([]listResponse, 0, len(lists))
	for _, list := range lists {
		items := make([]itemResponse, 0, len(list.Items))
		for _, item := range list.Items {
			items = append(items, itemResponse{ID: item.ID.String(), ProductVariantID: item.ProductVariantID.String(), CreatedAt: item.CreatedAt.UTC().Format(stdhttp.TimeFormat)})
		}
		response = append(response, listResponse{ID: list.ID.String(), CategoryID: list.CategoryID.String(), Items: items})
	}
	return response
}

func (handler *Handler) Add(context *gin.Context) {
	owner, err := handler.owner(context)
	if err != nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid comparison owner"})
		return
	}
	variantID, err := uuid.Parse(context.Param("variant_id"))
	if err != nil || variantID == uuid.Nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid product variant id"})
		return
	}
	if err := handler.service.Add(context.Request.Context(), owner, variantID); err != nil {
		handleError(context, err)
		return
	}
	context.Status(stdhttp.StatusNoContent)
}

func (handler *Handler) Remove(context *gin.Context) {
	owner, err := handler.owner(context)
	if err != nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid comparison owner"})
		return
	}
	variantID, err := uuid.Parse(context.Param("variant_id"))
	if err != nil || variantID == uuid.Nil {
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid product variant id"})
		return
	}
	if err := handler.service.Remove(context.Request.Context(), owner, variantID); err != nil {
		handleError(context, err)
		return
	}
	context.Status(stdhttp.StatusNoContent)
}

func (handler *Handler) owner(context *gin.Context) (comparisonDomain.Owner, error) {
	owner, createdSession, err := cartowner.FromContext(context)
	if err != nil {
		return comparisonDomain.Owner{}, err
	}
	if owner.CustomerID != nil {
		return comparisonDomain.Owner{UserID: owner.CustomerID}, nil
	}
	if owner.SessionID == nil {
		return comparisonDomain.Owner{}, comparisonDomain.ErrInvalidOwner
	}
	if createdSession {
		cartowner.SetSessionCookie(context, *owner.SessionID, handler.secureCookies)
	}
	return comparisonDomain.Owner{SessionID: owner.SessionID}, nil
}

func handleError(context *gin.Context, err error) {
	switch {
	case errors.Is(err, comparisonDomain.ErrInvalidOwner), errors.Is(err, comparisonDomain.ErrInvalidVariant):
		context.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid comparison request"})
	case errors.Is(err, comparisonDomain.ErrVariantNotFound):
		context.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "product variant is unavailable"})
	case errors.Is(err, comparisonDomain.ErrComparisonAtLimit):
		context.AbortWithStatusJSON(stdhttp.StatusConflict, gin.H{"error": "comparison item limit reached"})
	default:
		context.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "comparison unavailable"})
	}
}

func RegisterRoutes(group *gin.RouterGroup, service comparisonDomain.Service, secureCookies bool) {
	if service == nil {
		return
	}
	handler := NewHandler(service, secureCookies)
	group.GET("", handler.List)
	group.POST("/:variant_id", handler.Add)
	group.DELETE("/:variant_id", handler.Remove)
}

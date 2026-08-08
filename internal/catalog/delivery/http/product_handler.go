// Package http exposes the clean Catalog API. It only maps HTTP to the
// application port; persistence and catalog rules stay outside Gin.
package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type ProductHandler struct{ service domain.ProductService }

func NewProductHandler(service domain.ProductService) *ProductHandler {
	return &ProductHandler{service: service}
}

type CreateProductRequest struct {
	CategoryID   *uuid.UUID                  `json:"category_id,omitempty"`
	Status       string                      `json:"status,omitempty"`
	Translations []ProductTranslationRequest `json:"translations" binding:"required,min=1"`
}

type ProductTranslationRequest struct {
	Locale      string `json:"locale" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug" binding:"required"`
}

type ProductResponse struct {
	ID           uuid.UUID                    `json:"id"`
	CategoryID   *uuid.UUID                   `json:"category_id,omitempty"`
	Status       string                       `json:"status"`
	Translations []ProductTranslationResponse `json:"translations"`
	Rating       *ProductRatingResponse       `json:"rating,omitempty"`
}

type ProductRatingResponse struct {
	ReviewCount   int     `json:"review_count"`
	AverageRating float64 `json:"average_rating"`
}

type ProductTranslationResponse struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug"`
}

func (h *ProductHandler) Create(c *gin.Context) {
	var request CreateProductRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid request", "message": err.Error()})
		return
	}
	product := &domain.Product{CategoryID: request.CategoryID, Status: request.Status, Translations: make([]domain.ProductTranslation, 0, len(request.Translations))}
	for _, translation := range request.Translations {
		product.Translations = append(product.Translations, domain.ProductTranslation{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	if err := h.service.Create(c.Request.Context(), product); err != nil {
		handleProductError(c, err)
		return
	}
	c.JSON(stdhttp.StatusCreated, mapProduct(product))
}

func (h *ProductHandler) GetBySlug(c *gin.Context) {
	product, err := h.service.FindBySlug(c.Request.Context(), middleware.GetLanguage(c), c.Param("slug"))
	if err != nil {
		handleProductError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, mapProduct(product))
}

func handleProductError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrProductNotFound):
		c.JSON(stdhttp.StatusNotFound, gin.H{"error": "product not found"})
	case errors.Is(err, domain.ErrInvalidProduct):
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid product", "message": err.Error()})
	default:
		c.JSON(stdhttp.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func mapProduct(product *domain.Product) ProductResponse {
	translations := make([]ProductTranslationResponse, 0, len(product.Translations))
	for _, translation := range product.Translations {
		translations = append(translations, ProductTranslationResponse{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	response := ProductResponse{ID: product.ID, CategoryID: product.CategoryID, Status: product.Status, Translations: translations}
	if product.Rating != nil {
		response.Rating = &ProductRatingResponse{ReviewCount: product.Rating.ReviewCount, AverageRating: float64(product.Rating.AverageHundredths) / 100}
	}
	return response
}

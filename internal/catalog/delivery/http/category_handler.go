package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

type CategoryHandler struct{ service domain.CategoryService }

func NewCategoryHandler(service domain.CategoryService) *CategoryHandler {
	return &CategoryHandler{service: service}
}

type CreateCatalogCategoryRequest struct {
	ParentID     *uuid.UUID                   `json:"parent_id,omitempty"`
	SortOrder    int                          `json:"sort_order"`
	IsActive     *bool                        `json:"is_active,omitempty"`
	Translations []CategoryTranslationRequest `json:"translations" binding:"required,min=1"`
}

type CategoryTranslationRequest struct {
	Locale      string `json:"locale" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug" binding:"required"`
}

type CatalogCategoryResponse struct {
	ID           uuid.UUID                     `json:"id"`
	ParentID     *uuid.UUID                    `json:"parent_id,omitempty"`
	SortOrder    int                           `json:"sort_order"`
	IsActive     bool                          `json:"is_active"`
	Translations []CategoryTranslationResponse `json:"translations"`
}

type CategoryTranslationResponse struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Slug        string `json:"slug"`
}

func (h *CategoryHandler) Create(c *gin.Context) {
	var request CreateCatalogCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid request", "message": err.Error()})
		return
	}
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	category := &domain.Category{ParentID: request.ParentID, SortOrder: request.SortOrder, IsActive: isActive, Translations: make([]domain.CategoryTranslation, 0, len(request.Translations))}
	for _, translation := range request.Translations {
		category.Translations = append(category.Translations, domain.CategoryTranslation{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	if err := h.service.Create(c.Request.Context(), category); err != nil {
		handleCategoryError(c, err)
		return
	}
	c.JSON(stdhttp.StatusCreated, mapCategory(category))
}

func (h *CategoryHandler) GetBySlug(c *gin.Context) {
	category, err := h.service.FindBySlug(c.Request.Context(), middleware.GetLanguage(c), c.Param("slug"))
	if err != nil {
		handleCategoryError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, mapCategory(category))
}

func handleCategoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrCatalogCategoryNotFound):
		c.JSON(stdhttp.StatusNotFound, gin.H{"error": "category not found"})
	case errors.Is(err, domain.ErrInvalidCatalogCategory):
		c.JSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid category", "message": err.Error()})
	default:
		c.JSON(stdhttp.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}

func mapCategory(category *domain.Category) CatalogCategoryResponse {
	translations := make([]CategoryTranslationResponse, 0, len(category.Translations))
	for _, translation := range category.Translations {
		translations = append(translations, CategoryTranslationResponse{
			Locale: translation.Locale, Name: translation.Name, Description: translation.Description, Slug: translation.Slug,
		})
	}
	return CatalogCategoryResponse{ID: category.ID, ParentID: category.ParentID, SortOrder: category.SortOrder, IsActive: category.IsActive, Translations: translations}
}

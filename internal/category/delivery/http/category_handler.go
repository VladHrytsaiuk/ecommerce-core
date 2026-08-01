package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware" // аліас через конфлікт імен
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
)

type CategoryHandler struct {
	service  domain.CategoryService
	redirect redirectDomain.RedirectService
	l        logger.Logger
}

func NewCategoryHandler(s domain.CategoryService, redirect redirectDomain.RedirectService, l logger.Logger) *CategoryHandler {
	return &CategoryHandler{service: s, redirect: redirect, l: l}
}

// GetCategories godoc
// @Summary      Get all categories
// @Description  Get a flat list of all product categories in the requested language
// @Tags         Products
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        show_all query boolean false "Show all categories, including empty ones"
// @Success      200 {array}  CategoryResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/{lang}/categories [get]
func (h *CategoryHandler) GetCategories(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	showAll := c.Query("show_all") == "true"

	categories, err := h.service.GetList(c.Request.Context(), lang, showAll)
	if err != nil {
		h.l.Errorw("Failed to get categories list", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Could not retrieve categories",
		})
		return
	}

	c.JSON(http.StatusOK, mapCategoryListToResponse(categories, lang))
}

// GetCategoryBySlug godoc
// @Summary      Get category by slug
// @Description  Get a single category by slug for SEO purposes and 301 checks
// @Tags         Categories
// @Accept       json
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        slug path string true "Category slug"
// @Success      200 {object} CategoryResponse
// @Failure      404 {object} ErrorResponse
// @Failure      500 {object} ErrorResponse
// @Router       /api/{lang}/categories/by-slug/{slug} [get]
func (h *CategoryHandler) GetCategoryBySlug(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	slug := c.Param("slug")

	if slug == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Slug is required"})
		return
	}

	cat, err := h.service.GetBySlug(c.Request.Context(), slug, lang)
	if err != nil {
		if errors.Is(err, domain.ErrCategoryNotFound) {
			if h.redirect != nil {
				entityType, newSlug, redirectErr := h.redirect.ResolveRedirect(c.Request.Context(), slug, lang)
				if redirectErr == nil && newSlug != "" {
					prefix := ""
					if lang != "uk" {
						prefix = "/" + lang
					}
					c.Header("Location", prefix+"/category/"+newSlug)
					c.JSON(http.StatusMovedPermanently, gin.H{
						"error":       "Moved Permanently",
						"redirect_to": newSlug,
						"entity_type": entityType,
					})
					return
				}
			}

			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Category not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapCategoryToResponse(*cat, lang))
}

// CreateCategory godoc
// @Summary      Create category
// @Description  Create a new product category (Admin only)
// @Tags         Admin Categories
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body CreateCategoryRequest true "Category body"
// @Success      201      {object}  CategoryResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/categories [post]
func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	var iconURL *string
	if req.IconURL != nil && *req.IconURL != "" {
		iconURL = req.IconURL
	}

	var sortOrder int
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}

	category := &domain.Category{
		ParentID:  req.ParentID,
		IconURL:   iconURL,
		SortOrder: sortOrder,
		Translations: []domain.CategoryTranslation{
			{LanguageCode: "uk", Name: req.NameUk, Slug: req.Slug, MetaTitle: req.MetaTitleUk, MetaDescription: req.MetaDescriptionUk, MetaKeywords: req.MetaKeywordsUk},
			{LanguageCode: "en", Name: req.NameEn, MetaTitle: req.MetaTitleEn, MetaDescription: req.MetaDescriptionEn, MetaKeywords: req.MetaKeywordsEn},
		},
	}

	if err := h.service.Create(c.Request.Context(), category); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, mapCategoryToResponse(*category, "uk"))
}

// UpdateCategory godoc
// @Summary      Update category
// @Description  Update an existing category (Admin only)
// @Tags         Admin Categories
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path string true "Category UUID"
// @Param        body body UpdateCategoryRequest true "Category body"
// @Success      200      {object}  CategoryResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/categories/{id} [patch]
func (h *CategoryHandler) UpdateCategory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	// Зчитуємо тіло запиту для подальшого аналізу ключів (null vs absent для parent_id)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Failed to read request body"})
		return
	}

	var req UpdateCategoryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	category, err := h.service.GetByID(c.Request.Context(), id, "all")
	if err != nil {
		if errors.Is(err, domain.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Category not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	// Обробка ParentID: розрізняємо "не надіслано" та "надіслано як null" через raw JSON
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawFields); err == nil {
		if _, parentIDSent := rawFields["parent_id"]; parentIDSent {
			category.ParentID = req.ParentID // nil якщо null, UUID якщо задано
		}
		if _, slugSent := rawFields["slug"]; slugSent {
			if req.Slug != nil {
				for i := range category.Translations {
					if category.Translations[i].LanguageCode == "uk" {
						category.Translations[i].Slug = *req.Slug
						break
					}
				}
			}
		}
		if _, iconURLSent := rawFields["icon_url"]; iconURLSent {
			if req.IconURL != nil && *req.IconURL == "" {
				category.IconURL = nil
			} else {
				category.IconURL = req.IconURL // nil якщо null, string якщо задано
			}
		}
		if _, sortOrderSent := rawFields["sort_order"]; sortOrderSent {
			if req.SortOrder != nil {
				category.SortOrder = *req.SortOrder
			}
		}
	}

	// Обробка назв та мета-тегів
	if req.NameUk != nil || req.NameEn != nil || req.MetaTitleUk != nil || req.MetaTitleEn != nil ||
		req.MetaDescriptionUk != nil || req.MetaDescriptionEn != nil || req.MetaKeywordsUk != nil || req.MetaKeywordsEn != nil {
		newTranslations := []domain.CategoryTranslation{}
		ukFound, enFound := false, false
		for _, t := range category.Translations {
			if t.LanguageCode == "uk" {
				if req.NameUk != nil { t.Name = *req.NameUk }
				if req.MetaTitleUk != nil { t.MetaTitle = *req.MetaTitleUk }
				if req.MetaDescriptionUk != nil { t.MetaDescription = *req.MetaDescriptionUk }
				if req.MetaKeywordsUk != nil { t.MetaKeywords = *req.MetaKeywordsUk }
				ukFound = true
			} else if t.LanguageCode == "en" {
				if req.NameEn != nil { t.Name = *req.NameEn }
				if req.MetaTitleEn != nil { t.MetaTitle = *req.MetaTitleEn }
				if req.MetaDescriptionEn != nil { t.MetaDescription = *req.MetaDescriptionEn }
				if req.MetaKeywordsEn != nil { t.MetaKeywords = *req.MetaKeywordsEn }
				enFound = true
			}
			newTranslations = append(newTranslations, t)
		}

		if !ukFound {
			t := domain.CategoryTranslation{LanguageCode: "uk", CategoryID: id}
			if req.NameUk != nil { t.Name = *req.NameUk }
			if req.MetaTitleUk != nil { t.MetaTitle = *req.MetaTitleUk }
			if req.MetaDescriptionUk != nil { t.MetaDescription = *req.MetaDescriptionUk }
			if req.MetaKeywordsUk != nil { t.MetaKeywords = *req.MetaKeywordsUk }
			if t.Name != "" || t.MetaTitle != "" || t.MetaDescription != "" || t.MetaKeywords != "" {
				newTranslations = append(newTranslations, t)
			}
		}
		if !enFound {
			t := domain.CategoryTranslation{LanguageCode: "en", CategoryID: id}
			if req.NameEn != nil { t.Name = *req.NameEn }
			if req.MetaTitleEn != nil { t.MetaTitle = *req.MetaTitleEn }
			if req.MetaDescriptionEn != nil { t.MetaDescription = *req.MetaDescriptionEn }
			if req.MetaKeywordsEn != nil { t.MetaKeywords = *req.MetaKeywordsEn }
			if t.Name != "" || t.MetaTitle != "" || t.MetaDescription != "" || t.MetaKeywords != "" {
				newTranslations = append(newTranslations, t)
			}
		}
		category.Translations = newTranslations
	}

	if err := h.service.Update(c.Request.Context(), category); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapCategoryToResponse(*category, "uk"))
}

// DeleteCategory godoc
// @Summary      Delete category
// @Description  Delete a category (Admin only)
// @Tags         Admin Categories
// @Security     bearerAuth
// @Param        id path string true "Category UUID"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/categories/{id} [delete]
func (h *CategoryHandler) DeleteCategory(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrCategoryInUse) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category successfully deleted"})
}

// ReorderCategories godoc
// @Summary      Reorder categories
// @Description  Update sort_order for multiple categories at once (Admin only). The order of IDs in the array defines the new sequence (0, 1, 2...).
// @Tags         Admin Categories
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body ReorderCategoriesRequest true "IDs in desired order. Example: {'ids': ['uuid1', 'uuid2']}"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/categories/reorder [patch]
func (h *CategoryHandler) ReorderCategories(c *gin.Context) {
	var req ReorderCategoriesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
		return
	}

	if err := h.service.UpdateOrder(c.Request.Context(), req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Categories reordered successfully"})
}

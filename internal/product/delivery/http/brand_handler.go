package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
)

type BrandHandler struct {
	service domain.BrandService
	l       logger.Logger
}

func NewBrandHandler(s domain.BrandService, l logger.Logger) *BrandHandler {
	return &BrandHandler{service: s, l: l}
}

// GetBrands godoc
// @Summary      Get brands
// @Description  Get a list of all active brands
// @Tags         Products
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Success      200      {array}  BrandResponse
// @Failure      500      {object} ErrorResponse
// @Router       /api/{lang}/brands [get]
func (h *BrandHandler) GetBrands(c *gin.Context) {
	brands, err := h.service.GetAll(c.Request.Context())
	if err != nil {
		h.l.Errorw("Failed to get brands list", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, mapBrandListToResponse(brands))
}

// CreateBrand godoc
// @Summary      Create brand
// @Description  Create a new brand (Admin only)
// @Tags         Admin Brands
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body CreateBrandRequest true "Brand body"
// @Success      201      {object}  BrandResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/brands [post]
func (h *BrandHandler) CreateBrand(c *gin.Context) {
	var req CreateBrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	brand := &domain.Brand{
		Slug: req.Slug,
		Name: req.Name,
	}

	if err := h.service.Create(c.Request.Context(), brand); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, BrandResponse{ID: brand.ID, Slug: brand.Slug, Name: brand.Name})
}

// UpdateBrand godoc
// @Summary      Update brand
// @Description  Update an existing brand (Admin only)
// @Tags         Admin Brands
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path string true "Brand UUID"
// @Param        body body UpdateBrandRequest true "Brand body"
// @Success      200      {object}  BrandResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/brands/{id} [patch]
func (h *BrandHandler) UpdateBrand(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	var req UpdateBrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	brand, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrBrandNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Brand not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	if req.Name != nil {
		brand.Name = *req.Name
	}
	if req.Slug != nil {
		brand.Slug = *req.Slug
	}

	if err := h.service.Update(c.Request.Context(), brand); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, BrandResponse{ID: brand.ID, Slug: brand.Slug, Name: brand.Name})
}

// DeleteBrand godoc
// @Summary      Delete brand
// @Description  Delete a brand (Admin only)
// @Tags         Admin Brands
// @Security     bearerAuth
// @Param        id path string true "Brand UUID"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/brands/{id} [delete]
func (h *BrandHandler) DeleteBrand(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrBrandInUse) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Brand successfully deleted"})
}

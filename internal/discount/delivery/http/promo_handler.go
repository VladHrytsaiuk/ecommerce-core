package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

// PromoHandler обробляє HTTP запити для керування промокодами
type PromoHandler struct {
	service domain.PromoService
	l       logger.Logger
}

// NewPromoHandler створює новий інстанс хендлера
func NewPromoHandler(s domain.PromoService, l logger.Logger) *PromoHandler {
	return &PromoHandler{service: s, l: l}
}

// CreatePromo godoc
// @Summary      Create a new promo code
// @Description  Create a new promotional code with specific rules
// @Tags         Admin Promo
// @Accept       json
// @Produce      json
// @Param        body body CreatePromoRequest true "Promo details"
// @Security     bearerAuth
// @Success      201  {object} PromoResponse
// @Failure      400  {object} ErrorResponse
// @Failure      401  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/promos [post]
func (h *PromoHandler) CreatePromo(c *gin.Context) {
	var req CreatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	promo := &domain.PromoCode{
		Code:              req.Code,
		Description:       req.Description,
		DiscountType:      domain.DiscountType(req.DiscountType),
		DiscountValue:     req.DiscountValue,
		MinOrderSubtotal:  req.MinOrderSubtotal,
		UsageLimit:        req.UsageLimit,
		UsageLimitPerUser: req.UsageLimitPerUser,
		StartsAt:          req.StartsAt,
		EndsAt:            req.EndsAt,
		IsActive:          req.IsActive,
		CategoryIDs:       req.CategoryIDs,
		BrandIDs:          req.BrandIDs,
		ProductIDs:        req.ProductIDs,
	}

	createdPromo, err := h.service.CreatePromo(c.Request.Context(), promo)
	if err != nil {
		h.l.Errorw("failed to create promo code", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to create promo code",
		})
		return
	}

	c.JSON(http.StatusCreated, mapToPromoResponse(createdPromo))
}

// UpdatePromo godoc
// @Summary      Update promo code
// @Description  Update an existing promo code
// @Tags         Admin Promo
// @Accept       json
// @Produce      json
// @Param        id path string true "Promo ID"
// @Param        body body UpdatePromoRequest true "Update fields"
// @Security     bearerAuth
// @Success      200  {object} PromoResponse
// @Failure      400  {object} ErrorResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/promos/{id} [put]
func (h *PromoHandler) UpdatePromo(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid ID format",
		})
		return
	}

	var req UpdatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	promo, err := h.service.GetPromoByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrPromoCodeNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Promo code not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to fetch promo code",
		})
		return
	}

	if req.Description != nil {
		promo.Description = *req.Description
	}
	if req.DiscountType != nil {
		promo.DiscountType = domain.DiscountType(*req.DiscountType)
	}
	if req.DiscountValue != nil {
		promo.DiscountValue = *req.DiscountValue
	}
	if req.MinOrderSubtotal != nil {
		promo.MinOrderSubtotal = *req.MinOrderSubtotal
	}
	if req.UsageLimit != nil {
		promo.UsageLimit = req.UsageLimit
	}
	if req.UsageLimitPerUser != nil {
		promo.UsageLimitPerUser = req.UsageLimitPerUser
	}
	if req.StartsAt != nil {
		promo.StartsAt = req.StartsAt
	}
	if req.EndsAt != nil {
		promo.EndsAt = req.EndsAt
	}
	if req.IsActive != nil {
		promo.IsActive = *req.IsActive
	}
	if req.CategoryIDs != nil {
		promo.CategoryIDs = *req.CategoryIDs
	}
	if req.BrandIDs != nil {
		promo.BrandIDs = *req.BrandIDs
	}
	if req.ProductIDs != nil {
		promo.ProductIDs = *req.ProductIDs
	}

	updatedPromo, err := h.service.UpdatePromo(c.Request.Context(), id, promo)
	if err != nil {
		h.l.Errorw("failed to update promo code", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to update promo code",
		})
		return
	}

	c.JSON(http.StatusOK, mapToPromoResponse(updatedPromo))
}

// GetPromo godoc
// @Summary      Get promo code by ID
// @Description  Returns promo code details including relations
// @Tags         Admin Promo
// @Produce      json
// @Param        id path string true "Promo ID"
// @Security     bearerAuth
// @Success      200  {object} PromoResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/promos/{id} [get]
func (h *PromoHandler) GetPromo(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid ID format",
		})
		return
	}

	promo, err := h.service.GetPromoByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrPromoCodeNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "Not Found",
				Message: "Promo code not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to fetch promo code",
		})
		return
	}

	c.JSON(http.StatusOK, mapToPromoResponse(promo))
}

// ListPromos godoc
// @Summary      List promo codes
// @Description  Get a paginated list of promo codes
// @Tags         Admin Promo
// @Produce      json
// @Param        page query int false "Page number"
// @Param        per_page query int false "Items per page"
// @Security     bearerAuth
// @Success      200  {object} map[string]interface{}
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/promos [get]
func (h *PromoHandler) ListPromos(c *gin.Context) {
	var pgn pagination.Params
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: validationerrors.FormatValidationError(err),
		})
		return
	}

	promos, meta, err := h.service.ListPromos(c.Request.Context(), pgn)
	if err != nil {
		h.l.Errorw("failed to list promo codes", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to list promo codes",
		})
		return
	}

	resp := make([]PromoResponse, len(promos))
	for i, p := range promos {
		resp[i] = mapToPromoResponse(&p)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": resp,
		"meta": meta,
	})
}

// DeletePromo godoc
// @Summary      Delete promo code
// @Description  Delete a promo code
// @Tags         Admin Promo
// @Produce      json
// @Param        id path string true "Promo ID"
// @Security     bearerAuth
// @Success      200  {object} map[string]string
// @Failure      400  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /api/admin/promos/{id} [delete]
func (h *PromoHandler) DeletePromo(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid ID format",
		})
		return
	}

	err = h.service.DeletePromo(c.Request.Context(), id)
	if err != nil {
		h.l.Errorw("failed to delete promo code", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Internal Server Error",
			Message: "Failed to delete promo code",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Promo code deleted successfully"})
}

// ErrorResponse represents standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
)

type BadgeHandler struct {
	service domain.BadgeService
	l       logger.Logger
}

func NewBadgeHandler(s domain.BadgeService, l logger.Logger) *BadgeHandler {
	return &BadgeHandler{service: s, l: l}
}

// GetBadges godoc
// @Summary      Get badges
// @Description  Get a list of all available badges
// @Tags         Badges
// @Produce      json
// @Success      200      {array}  AdminBadgeResponse
// @Failure      500      {object} ErrorResponse
// @Router       /api/admin/badges [get]
func (h *BadgeHandler) GetBadges(c *gin.Context) {
	badges, err := h.service.GetAll(c.Request.Context())
	if err != nil {
		h.l.Errorw("Failed to get badges list", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, mapBadgeListToAdminResponse(badges))
}

// CreateBadge godoc
// @Summary      Create badge
// @Description  Create a new badge (Admin only)
// @Tags         Admin Badges
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body CreateBadgeRequest true "Badge body"
// @Success      201      {object}  AdminBadgeResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/badges [post]
func (h *BadgeHandler) CreateBadge(c *gin.Context) {
	var req CreateBadgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	badge := &domain.Badge{
		Name:      domain.LocalizedMap{"uk": req.NameUk, "en": req.NameEn},
		ColorHex:  req.ColorHex,
		SortOrder: req.SortOrder,
	}

	if err := h.service.Create(c.Request.Context(), badge); err != nil {
		h.l.Errorw("Failed to create badge", "err", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create badge"})
		return
	}

	c.JSON(http.StatusCreated, mapBadgeToAdminResponse(*badge))
}

// UpdateBadge godoc
// @Summary      Update badge
// @Description  Update an existing badge (Admin only)
// @Tags         Admin Badges
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id   path int true "Badge ID"
// @Param        body body UpdateBadgeRequest true "Badge body"
// @Success      200      {object}  AdminBadgeResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      404      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/badges/{id} [patch]
func (h *BadgeHandler) UpdateBadge(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	var req UpdateBadgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: validationerrors.FormatValidationError(err)})
		return
	}

	badge, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrBadgeNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not Found", Message: "Badge not found"})
			return
		}
		h.l.Errorw("Failed to get badge", "err", err, "id", id)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal Server Error"})
		return
	}

	if req.NameUk != nil {
		badge.Name["uk"] = *req.NameUk
	}
	if req.NameEn != nil {
		badge.Name["en"] = *req.NameEn
	}
	if req.ColorHex != nil {
		badge.ColorHex = *req.ColorHex
	}
	if req.SortOrder != nil {
		badge.SortOrder = *req.SortOrder
	}

	if err := h.service.Update(c.Request.Context(), badge); err != nil {
		h.l.Errorw("Failed to update badge", "err", err, "id", id)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update badge"})
		return
	}

	c.JSON(http.StatusOK, mapBadgeToAdminResponse(*badge))
}

// DeleteBadge godoc
// @Summary      Delete badge
// @Description  Delete a badge (Admin only). Fails if badge is assigned to products or variations.
// @Tags         Admin Badges
// @Security     bearerAuth
// @Param        id path int true "Badge ID"
// @Success      200      {object}  map[string]string
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /api/admin/badges/{id} [delete]
func (h *BadgeHandler) DeleteBadge(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Invalid ID"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrBadgeInUse) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Bad Request", Message: "Badge is assigned to products and cannot be deleted"})
			return
		}
		h.l.Errorw("Failed to delete badge", "err", err, "id", id)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete badge"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Badge successfully deleted"})
}

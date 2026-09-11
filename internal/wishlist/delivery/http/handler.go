// Package http exposes the optional Wishlist module without leaking its
// persistence details into transport.
package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/cartowner"
	wishlistDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

type Handler struct {
	service       wishlistDomain.Service
	secureCookies bool
}

func NewHandler(service wishlistDomain.Service, secureCookies bool) *Handler {
	return &Handler{service: service, secureCookies: secureCookies}
}

// List godoc
// @Summary List the current buyer's wishlist
// @Description Scoped to the authenticated customer, or to the anonymous cart_session cookie.
// @Tags Wishlist
// @Produce json
// @Success 200 {object} map[string][]string
// @Failure 400 {object} map[string]string
// @Router /api/wishlist [get]
func (h *Handler) List(c *gin.Context) {
	owner, err := h.owner(c)
	if err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid wishlist owner"})
		return
	}
	wishlist, err := h.service.List(c.Request.Context(), owner)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, gin.H{"items": wishlist.Items})
}

// Add godoc
// @Summary Add a product variant to the wishlist
// @Tags Wishlist
// @Produce json
// @Param variant_id path string true "Product variant UUID"
// @Success 204 "No Content"
// @Failure 400,404,422 {object} map[string]string
// @Router /api/wishlist/{variant_id} [post]
func (h *Handler) Add(c *gin.Context) {
	owner, err := h.owner(c)
	if err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid wishlist owner"})
		return
	}
	variantID, err := uuid.Parse(c.Param("variant_id"))
	if err != nil || variantID == uuid.Nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid product variant id"})
		return
	}
	if err := h.service.Add(c.Request.Context(), owner, variantID); err != nil {
		handleError(c, err)
		return
	}
	c.Status(stdhttp.StatusNoContent)
}

// Remove godoc
// @Summary Remove a product variant from the wishlist
// @Tags Wishlist
// @Produce json
// @Param variant_id path string true "Product variant UUID"
// @Success 204 "No Content"
// @Failure 400,404 {object} map[string]string
// @Router /api/wishlist/{variant_id} [delete]
func (h *Handler) Remove(c *gin.Context) {
	owner, err := h.owner(c)
	if err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid wishlist owner"})
		return
	}
	variantID, err := uuid.Parse(c.Param("variant_id"))
	if err != nil || variantID == uuid.Nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid product variant id"})
		return
	}
	if err := h.service.Remove(c.Request.Context(), owner, variantID); err != nil {
		handleError(c, err)
		return
	}
	c.Status(stdhttp.StatusNoContent)
}

func (h *Handler) owner(c *gin.Context) (wishlistDomain.Owner, error) {
	owner, createdSession, err := cartowner.FromContext(c)
	if err != nil {
		return wishlistDomain.Owner{}, err
	}
	if owner.CustomerID != nil {
		return wishlistDomain.Owner{UserID: owner.CustomerID}, nil
	}
	if owner.SessionID == nil {
		return wishlistDomain.Owner{}, wishlistDomain.ErrInvalidOwner
	}
	if createdSession {
		cartowner.SetSessionCookie(c, *owner.SessionID, h.secureCookies)
	}
	return wishlistDomain.Owner{SessionID: owner.SessionID}, nil
}

func handleError(c *gin.Context, err error) {
	if errors.Is(err, wishlistDomain.ErrInvalidOwner) || errors.Is(err, wishlistDomain.ErrInvalidVariant) {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid wishlist request"})
		return
	}
	c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "wishlist unavailable"})
}

func RegisterRoutes(group *gin.RouterGroup, service wishlistDomain.Service, secureCookies bool) {
	if service == nil {
		return
	}
	handler := NewHandler(service, secureCookies)
	group.GET("", handler.List)
	group.POST("/:variant_id", handler.Add)
	group.DELETE("/:variant_id", handler.Remove)
}

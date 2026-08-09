package http

import (
	"errors"
	stdhttp "net/http"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ service domain.Service }

func NewHandler(service domain.Service) *Handler { return &Handler{service: service} }

type request struct {
	Slug         string               `json:"slug"`
	Color        string               `json:"color"`
	Translations []domain.Translation `json:"translations"`
}

func (h *Handler) Create(c *gin.Context) {
	var request request
	if c.ShouldBindJSON(&request) != nil {
		c.AbortWithStatusJSON(400, gin.H{"error": "invalid badge"})
		return
	}
	badge, err := h.service.Create(c.Request.Context(), domain.CreateCommand{Slug: request.Slug, Color: request.Color, Translations: request.Translations})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusCreated, badge)
}
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseID(c, "badge_id")
	if !ok {
		return
	}
	badge, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, badge)
}
func (h *Handler) List(c *gin.Context) {
	badges, err := h.service.List(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, gin.H{"badges": badges})
}
func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c, "badge_id")
	if !ok {
		return
	}
	var request request
	if c.ShouldBindJSON(&request) != nil {
		c.AbortWithStatusJSON(400, gin.H{"error": "invalid badge"})
		return
	}
	badge, err := h.service.Update(c.Request.Context(), id, domain.UpdateCommand{Slug: request.Slug, Color: request.Color, Translations: request.Translations})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, badge)
}
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c, "badge_id")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}
	c.Status(stdhttp.StatusNoContent)
}
func (h *Handler) Assign(c *gin.Context) {
	badgeID, ok := parseID(c, "badge_id")
	if !ok {
		return
	}
	productID, ok := parseID(c, "product_id")
	if !ok {
		return
	}
	if err := h.service.AssignProduct(c.Request.Context(), badgeID, productID); err != nil {
		handleError(c, err)
		return
	}
	c.Status(stdhttp.StatusNoContent)
}
func (h *Handler) Remove(c *gin.Context) {
	badgeID, ok := parseID(c, "badge_id")
	if !ok {
		return
	}
	productID, ok := parseID(c, "product_id")
	if !ok {
		return
	}
	if err := h.service.RemoveProduct(c.Request.Context(), badgeID, productID); err != nil {
		handleError(c, err)
		return
	}
	c.Status(stdhttp.StatusNoContent)
}
func parseID(c *gin.Context, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(key))
	if err != nil || id == uuid.Nil {
		c.AbortWithStatusJSON(400, gin.H{"error": "invalid identifier"})
		return uuid.Nil, false
	}
	return id, true
}
func handleError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrInvalidBadge) {
		c.AbortWithStatusJSON(400, gin.H{"error": "invalid badge"})
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		c.AbortWithStatusJSON(404, gin.H{"error": "badge not found"})
		return
	}
	c.AbortWithStatusJSON(500, gin.H{"error": "badges unavailable"})
}
func RegisterRoutes(admin *gin.RouterGroup, service domain.Service) {
	if service == nil {
		return
	}
	h := NewHandler(service)
	group := admin.Group("/badges")
	group.GET("", h.List)
	group.POST("", h.Create)
	group.GET("/:badge_id", h.Get)
	group.PUT("/:badge_id", h.Update)
	group.DELETE("/:badge_id", h.Delete)
	group.PUT("/:badge_id/products/:product_id", h.Assign)
	group.DELETE("/:badge_id/products/:product_id", h.Remove)
}

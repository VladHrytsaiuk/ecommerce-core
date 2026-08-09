package http

import (
	"errors"
	stdhttp "net/http"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ service domain.Service }

func NewHandler(service domain.Service) *Handler { return &Handler{service: service} }

type upsertRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Locale       string `json:"locale"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Keywords     string `json:"keywords"`
	OGImageRef   string `json:"og_image_ref"`
}

func (h *Handler) Upsert(c *gin.Context) {
	var request upsertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid SEO metadata"})
		return
	}
	id, err := uuid.Parse(request.ResourceID)
	if err != nil || id == uuid.Nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid resource_id"})
		return
	}
	metadata, err := h.service.Upsert(c.Request.Context(), domain.UpsertCommand{ResourceType: request.ResourceType, ResourceID: id, Locale: request.Locale, Title: request.Title, Description: request.Description, Keywords: request.Keywords, OGImageRef: request.OGImageRef})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, metadata)
}
func (h *Handler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("resource_id"))
	if err != nil || id == uuid.Nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid resource_id"})
		return
	}
	metadata, err := h.service.Get(c.Request.Context(), c.Param("resource_type"), id, c.Param("locale"))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(stdhttp.StatusOK, metadata)
}
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("resource_id"))
	if err != nil || id == uuid.Nil {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid resource_id"})
		return
	}
	if err = h.service.Delete(c.Request.Context(), c.Param("resource_type"), id, c.Param("locale")); err != nil {
		handleError(c, err)
		return
	}
	c.Status(stdhttp.StatusNoContent)
}
func handleError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrInvalidMetadata) {
		c.AbortWithStatusJSON(stdhttp.StatusBadRequest, gin.H{"error": "invalid SEO metadata"})
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		c.AbortWithStatusJSON(stdhttp.StatusNotFound, gin.H{"error": "SEO metadata not found"})
		return
	}
	c.AbortWithStatusJSON(stdhttp.StatusInternalServerError, gin.H{"error": "SEO unavailable"})
}
func RegisterRoutes(admin *gin.RouterGroup, service domain.Service) {
	if service == nil {
		return
	}
	handler := NewHandler(service)
	group := admin.Group("/seo")
	group.GET("/:resource_type/:resource_id/:locale", handler.Get)
	group.PUT("", handler.Upsert)
	group.DELETE("/:resource_type/:resource_id/:locale", handler.Delete)
}

var _ = strings.TrimSpace

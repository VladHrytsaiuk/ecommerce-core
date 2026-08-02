//go:build legacy
// +build legacy

package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/document/domain"
	mymiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	validationerrors "github.com/VladHrytsaiuk/ecommerce-core/internal/shared/errors"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DocumentHandler struct {
	service domain.DocumentService
	l       logger.Logger
}

func NewDocumentHandler(service domain.DocumentService, l logger.Logger) *DocumentHandler {
	return &DocumentHandler{service: service, l: l}
}

// GetDocumentBySlug godoc
// @Summary      Get document by slug
// @Description  Get a document's localized active content by its slug
// @Tags         Documents
// @Produce      json
// @Param        lang path string true "Language code (uk, en)"
// @Param        slug path string true "Document slug"
// @Success      200  {object} PublicDocumentResponse
// @Failure      404  {object} map[string]string
// @Failure      500  {object} map[string]string
// @Router       /api/{lang}/documents/by-slug/{slug} [get]
func (h *DocumentHandler) GetDocumentBySlug(c *gin.Context) {
	lang := mymiddleware.GetLanguage(c)
	slug := c.Param("slug")

	doc, err := h.service.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		if errors.Is(err, domain.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
		h.l.Errorw("failed to get document by slug", "error", err, "slug", slug)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, mapPublicDocument(*doc, lang))
}

// ListDocuments godoc
// @Summary      List all documents (Admin)
// @Description  Get a list of documents and their active version info
// @Tags         Admin Documents
// @Produce      json
// @Security     bearerAuth
// @Param        page query int false "Page number"
// @Param        limit query int false "Items per page"
// @Success      200  {object} pagination.PagedResponse{data=[]DocumentListResponse}
// @Failure      500  {object} map[string]string
// @Router       /api/admin/documents [get]
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	var pgn pagination.Params
	if err := c.ShouldBindQuery(&pgn); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationerrors.FormatValidationError(err)})
		return
	}

	docs, meta, err := h.service.GetList(c.Request.Context(), pgn)
	if err != nil {
		h.l.Errorw("failed to list documents", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	data := make([]DocumentListResponse, len(docs))
	for i, doc := range docs {
		data[i] = mapDocumentList(doc)
	}

	c.JSON(http.StatusOK, pagination.PagedResponse{
		Data:     data,
		Metadata: meta,
	})
}

// GetDocumentByID godoc
// @Summary      Get document details (Admin)
// @Description  Get full document details including version history
// @Tags         Admin Documents
// @Produce      json
// @Security     bearerAuth
// @Param        id path string true "Document ID"
// @Success      200  {object} AdminDocumentDetailResponse
// @Failure      404  {object} map[string]string
// @Failure      500  {object} map[string]string
// @Router       /api/admin/documents/{id} [get]
func (h *DocumentHandler) GetDocumentByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document ID"})
		return
	}

	doc, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrDocumentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Document not found"})
			return
		}
		h.l.Errorw("failed to get document by id", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, mapAdminDocumentDetail(*doc))
}

// CreateDocument godoc
// @Summary      Create a document (Admin)
// @Description  Create a new document with an initial version
// @Tags         Admin Documents
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        body body CreateDocumentRequest true "Document body"
// @Success      201  {object} map[string]interface{}
// @Failure      400  {object} map[string]string
// @Failure      500  {object} map[string]string
// @Router       /api/admin/documents [post]
func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	var req CreateDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationerrors.FormatValidationError(err)})
		return
	}

	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var createdBy uuid.UUID
	switch v := userIDStr.(type) {
	case string:
		createdBy, _ = uuid.Parse(v)
	case uuid.UUID:
		createdBy = v
	}

	doc := &domain.Document{
		ID:    uuid.New(),
		Slug:  req.Slug,
		Title: req.Title,
	}

	contentBytes, err := json.Marshal(req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid content format"})
		return
	}

	err = h.service.Create(c.Request.Context(), doc, contentBytes, req.Changelog, createdBy)
	if err != nil {
		if errors.Is(err, domain.ErrSlugAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "Slug already exists"})
			return
		}
		h.l.Errorw("failed to create document", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":   doc.ID,
		"slug": doc.Slug,
	})
}

// SaveVersion godoc
// @Summary      Save a new document version (Admin)
// @Description  Add a new version to an existing document and make it active
// @Tags         Admin Documents
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id path string true "Document ID"
// @Param        body body SaveVersionRequest true "Version body"
// @Success      201  {object} map[string]string
// @Failure      400  {object} map[string]string
// @Failure      500  {object} map[string]string
// @Router       /api/admin/documents/{id}/versions [post]
func (h *DocumentHandler) SaveVersion(c *gin.Context) {
	idStr := c.Param("id")
	documentID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document ID"})
		return
	}

	var req SaveVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationerrors.FormatValidationError(err)})
		return
	}

	userIDStr, _ := c.Get("user_id")
	var createdBy uuid.UUID
	switch v := userIDStr.(type) {
	case string:
		createdBy, _ = uuid.Parse(v)
	case uuid.UUID:
		createdBy = v
	}

	contentBytes, err := json.Marshal(req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid content format"})
		return
	}

	err = h.service.SaveVersion(c.Request.Context(), documentID, contentBytes, req.Changelog, createdBy)
	if err != nil {
		h.l.Errorw("failed to save version", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Version saved successfully"})
}

// SetActiveVersion godoc
// @Summary      Set active version (Admin)
// @Description  Rollback/activate a specific document version
// @Tags         Admin Documents
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id path string true "Document ID"
// @Param        body body SetActiveVersionRequest true "Active version body"
// @Success      200  {object} map[string]string
// @Failure      400  {object} map[string]string
// @Failure      404  {object} map[string]string
// @Failure      500  {object} map[string]string
// @Router       /api/admin/documents/{id}/active-version [post]
func (h *DocumentHandler) SetActiveVersion(c *gin.Context) {
	idStr := c.Param("id")
	documentID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document ID"})
		return
	}

	var req SetActiveVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationerrors.FormatValidationError(err)})
		return
	}

	err = h.service.SetActiveVersion(c.Request.Context(), documentID, req.VersionID)
	if err != nil {
		if errors.Is(err, domain.ErrDocumentNotFound) || errors.Is(err, domain.ErrVersionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Document or Version not found"})
			return
		}
		h.l.Errorw("failed to set active version", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Active version set successfully"})
}

// DeleteDocument godoc
// @Summary      Delete document (Admin)
// @Description  Soft delete a document
// @Tags         Admin Documents
// @Produce      json
// @Security     bearerAuth
// @Param        id path string true "Document ID"
// @Success      200  {object} map[string]string
// @Failure      400  {object} map[string]string
// @Failure      500  {object} map[string]string
// @Router       /api/admin/documents/{id} [delete]
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document ID"})
		return
	}

	err = h.service.Delete(c.Request.Context(), id)
	if err != nil {
		h.l.Errorw("failed to delete document", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Document deleted successfully"})
}

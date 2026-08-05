//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/document/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

// RegisterDocumentRoutes registers the routes for the document module
func RegisterDocumentRoutes(langGroup *gin.RouterGroup, adminGroup *gin.RouterGroup, service domain.DocumentService, l logger.Logger) {
	h := NewDocumentHandler(service, l)

	// Public routes
	langGroup.GET("/documents/by-slug/:slug", h.GetDocumentBySlug)

	// Admin routes
	if adminGroup != nil {
		adminGroup.GET("/documents", h.ListDocuments)
		adminGroup.GET("/documents/:id", h.GetDocumentByID)
		adminGroup.POST("/documents", h.CreateDocument)
		adminGroup.POST("/documents/:id/versions", h.SaveVersion)
		adminGroup.POST("/documents/:id/active-version", h.SetActiveVersion)
		adminGroup.DELETE("/documents/:id", h.DeleteDocument)
	}
}

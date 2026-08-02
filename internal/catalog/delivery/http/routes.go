package http

import (
	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

// RegisterProductRoutes attaches the clean Catalog product contract. These
// paths intentionally do not reuse the predecessor product API contract.
func RegisterProductRoutes(localeGroup, adminGroup *gin.RouterGroup, service domain.ProductService) {
	handler := NewProductHandler(service)
	localeGroup.GET("/catalog/products/by-slug/:slug", handler.GetBySlug)
	adminGroup.POST("/catalog/products", handler.Create)
}

func RegisterVariantRoutes(adminGroup *gin.RouterGroup, service domain.VariantService) {
	handler := NewVariantHandler(service)
	adminGroup.POST("/catalog/variants", handler.Create)
}

func RegisterCategoryRoutes(localeGroup, adminGroup *gin.RouterGroup, service domain.CategoryService) {
	handler := NewCategoryHandler(service)
	localeGroup.GET("/catalog/categories/by-slug/:slug", handler.GetBySlug)
	adminGroup.POST("/catalog/categories", handler.Create)
}

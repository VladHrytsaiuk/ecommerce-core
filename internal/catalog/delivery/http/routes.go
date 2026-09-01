package http

import (
	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

// RegisterProductRoutes attaches the clean Catalog product contract. These
// paths intentionally do not reuse the predecessor product API contract.
func RegisterProductRoutes(localeGroup, adminGroup *gin.RouterGroup, service domain.ProductService) {
	handler := NewProductHandler(service)
	localeGroup.GET("/catalog/products", handler.List)
	localeGroup.GET("/catalog/products/by-slug/:slug", handler.GetBySlug)
	if adminGroup != nil {
		adminGroup.POST("/catalog/products", handler.Create)
	}
}

func RegisterVariantRoutes(adminGroup *gin.RouterGroup, service domain.VariantService) {
	handler := NewVariantHandler(service)
	if adminGroup != nil {
		adminGroup.POST("/catalog/variants", handler.Create)
	}
}

func RegisterCategoryRoutes(localeGroup, adminGroup *gin.RouterGroup, service domain.CategoryService) {
	handler := NewCategoryHandler(service)
	localeGroup.GET("/catalog/categories/by-slug/:slug", handler.GetBySlug)
	if adminGroup != nil {
		adminGroup.POST("/catalog/categories", handler.Create)
	}
}

// RegisterV1Routes attaches the additive Catalog v1 contract. Legacy catalog
// routes remain registered independently for existing clients.
func RegisterV1Routes(group *gin.RouterGroup, service domain.ProductService, renderer *apiresponse.ErrorRenderer, availability ...domain.VariantAvailabilityReader) {
	handler := NewCatalogV1Handler(service, renderer, availability...)
	group.GET("/products", handler.List)
	group.GET("/products/by-slug/:slug", handler.GetBySlug)
}

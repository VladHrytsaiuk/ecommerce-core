package http

import (
	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

// RegisterV1Routes attaches the Catalog storefront contract.
//
// A second, older registration used to sit alongside this one, serving the
// same products at /api/{lang}/catalog/products through a different handler.
// Its comment justified the duplication as compatibility for existing clients,
// but this core is copied to start a store, so there are no existing clients
// to be compatible with — only two implementations to keep in step, one of
// which had no OpenAPI annotations and did not consult inventory availability.
func RegisterV1Routes(group *gin.RouterGroup, products domain.ProductService, categories domain.CategoryService, renderer *apiresponse.ErrorRenderer, availability ...domain.VariantAvailabilityReader) {
	handler := NewCatalogV1Handler(products, renderer, availability...)
	group.GET("/products", handler.List)
	group.GET("/products/by-slug/:slug", handler.GetBySlug)
	if categories != nil {
		group.GET("/categories/by-slug/:slug", NewCategoryV1Handler(categories, renderer).GetBySlug)
	}
}

package http

import (
	"github.com/gin-gonic/gin"
	categoryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
)

// RegisterProductRoutes реєструє маршрути модуля товарів
func RegisterProductRoutes(rg *gin.RouterGroup, authRg *gin.RouterGroup, adminRg *gin.RouterGroup, apiRg *gin.RouterGroup, noLangAuthRg *gin.RouterGroup, s domain.ProductService, catSvc categoryDomain.CategoryService, redirect redirectDomain.RedirectService, l logger.Logger) {
	h := NewProductHandler(s, catSvc, redirect, l)

	rg.GET("/products", h.GetProducts)
	rg.GET("/products/recommended", h.GetRecommendedProducts)
	rg.GET("/products/search/quick", h.QuickSearchProducts)
	rg.GET("/products/filters", h.GetProductFilters)
	rg.GET("/products/by-slug/:slug", h.GetProductBySlug)
	rg.GET("/products/:id", h.GetProductByID)
	rg.GET("/units", h.GetUnits)

	// Reviews (без префіксу мови)
	apiRg.GET("/products/:id/reviews", h.GetProductReviews)

	// Auth required endpoints
	noLangAuthRg.POST("/products/:id/reviews", h.CreateReview)

	// Admin required endpoints
	if adminRg != nil {
		adminRg.POST("/products", h.CreateProduct)
		adminRg.PATCH("/products/:id", h.UpdateProduct)
		adminRg.DELETE("/products/:id", h.DeleteProduct)
		adminRg.PATCH("/reviews/:id/approve", h.ApproveReview)
		adminRg.PATCH("/reviews/:id/reject", h.RejectReview)
		adminRg.PATCH("/reviews/:id/reopen", h.ReopenReview)
		adminRg.POST("/products/:id/reviews", h.CreateReview)
		adminRg.GET("/reviews/pending", h.GetPendingReviews)
		adminRg.GET("/reviews/rejected", h.GetRejectedReviews)

		// Image management
		adminRg.POST("/products/:id/images", h.UploadProductImages)
		adminRg.DELETE("/products/:id/images/:img_id", h.DeleteProductImage)
		adminRg.PATCH("/products/:id/images/:img_id", h.UpdateProductImage)
		adminRg.PATCH("/products/:id/images/reorder", h.ReorderProductImages)

		// General media
		adminRg.POST("/media/upload", h.UploadMedia)
	}
}

// RegisterBrandRoutes реєструє маршрути брендів
func RegisterBrandRoutes(rg *gin.RouterGroup, adminRg *gin.RouterGroup, s domain.BrandService, l logger.Logger) {
	h := NewBrandHandler(s, l)
	rg.GET("/brands", h.GetBrands)

	if adminRg != nil {
		adminRg.POST("/brands", h.CreateBrand)
		adminRg.PATCH("/brands/:id", h.UpdateBrand)
		adminRg.DELETE("/brands/:id", h.DeleteBrand)
	}
}

// RegisterAttributeRoutes реєструє маршрути атрибутів
func RegisterAttributeRoutes(rg *gin.RouterGroup, adminRg *gin.RouterGroup, s domain.AttributeService, l logger.Logger) {
	h := NewAttributeHandler(s, l)
	rg.GET("/attributes", h.GetAttributes)

	if adminRg != nil {
		// Всі значення характеристики (напр. "type") для підказок у формі товару —
		// незалежно від активності товарів, на відміну від /products/filters (вітрина).
		adminRg.GET("/attributes/values", h.GetAttributeValues)
		adminRg.POST("/attributes", h.CreateAttribute)
		adminRg.PATCH("/attributes/:id", h.UpdateAttribute)
		adminRg.DELETE("/attributes/:id", h.DeleteAttribute)
		adminRg.PATCH("/attributes/reorder", h.ReorderAttributes)
	}
}

// RegisterBadgeRoutes реєструє маршрути бейджів
func RegisterBadgeRoutes(adminRg *gin.RouterGroup, s domain.BadgeService, l logger.Logger) {
	h := NewBadgeHandler(s, l)

	if adminRg != nil {
		adminRg.GET("/badges", h.GetBadges)
		adminRg.POST("/badges", h.CreateBadge)
		adminRg.PATCH("/badges/:id", h.UpdateBadge)
		adminRg.DELETE("/badges/:id", h.DeleteBadge)
	}
}

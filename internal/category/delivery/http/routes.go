package http

import (
	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	redirectDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
)

// RegisterCategoryRoutes реєструє маршрути модуля категорій
func RegisterCategoryRoutes(rg *gin.RouterGroup, adminRg *gin.RouterGroup, s domain.CategoryService, redirect redirectDomain.RedirectService, l logger.Logger) {
	h := NewCategoryHandler(s, redirect, l)

	rg.GET("/categories", h.GetCategories)
	rg.GET("/categories/by-slug/:slug", h.GetCategoryBySlug)

	if adminRg != nil {
		adminRg.POST("/categories", h.CreateCategory)
		adminRg.PATCH("/categories/reorder", h.ReorderCategories)
		adminRg.PATCH("/categories/:id", h.UpdateCategory)
		adminRg.DELETE("/categories/:id", h.DeleteCategory)
	}
}

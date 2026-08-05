//go:build !legacy

package http

import (
	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(group *gin.RouterGroup, service domain.Service, secureCookies bool) {
	if service == nil {
		return
	}
	h := NewHandler(service, secureCookies)
	group.GET("/cart", h.Get)
	group.POST("/cart/items", h.Add)
	group.PATCH("/cart/items/:variantID", h.SetQuantity)
	group.DELETE("/cart/items/:variantID", h.Remove)
}

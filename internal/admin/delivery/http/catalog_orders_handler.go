package http

import (
	adminApp "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	shared "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
)

func RegisterCatalogRoutes(g *gin.RouterGroup, a adminDomain.Authorizer, f *adminApp.CatalogAdminFacade) {
	if g == nil || f == nil {
		return
	}
	g.POST("/catalog/products", RequirePermission(a, adminApp.PermissionCatalogWrite), catalogProduct(f, false))
	g.PUT("/catalog/products/:id", RequirePermission(a, adminApp.PermissionCatalogWrite), catalogProduct(f, true))
	g.DELETE("/catalog/products/:id", RequirePermission(a, adminApp.PermissionCatalogWrite), deleteCatalogProduct(f))
	g.POST("/catalog/categories", RequirePermission(a, adminApp.PermissionCatalogWrite), catalogCategory(f, false))
	g.PUT("/catalog/categories/:id", RequirePermission(a, adminApp.PermissionCatalogWrite), catalogCategory(f, true))
}

func deleteCatalogProduct(f *adminApp.CatalogAdminFacade) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			c.Status(http.StatusUnauthorized)
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_ID"})
			return
		}
		if err := f.DeleteProduct(c, adminApp.CatalogCommand{ActorUserID: actor, EventKey: idempotency(c), IPAddress: c.ClientIP()}, id); err != nil {
			catalogError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
func catalogProduct(f *adminApp.CatalogAdminFacade, update bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			c.Status(401)
			return
		}
		var p catalogDomain.Product
		if err := c.ShouldBindJSON(&p); err != nil {
			c.JSON(400, gin.H{"error": "INVALID_REQUEST"})
			return
		}
		if update {
			id, err := uuid.Parse(c.Param("id"))
			if err != nil || id == uuid.Nil {
				c.JSON(400, gin.H{"error": "INVALID_ID"})
				return
			}
			p.ID = id
		}
		cmd := adminApp.CatalogCommand{ActorUserID: actor, EventKey: idempotency(c), IPAddress: c.ClientIP()}
		var err error
		if update {
			err = f.UpdateProduct(c, cmd, &p)
		} else {
			err = f.CreateProduct(c, cmd, &p)
		}
		if err != nil {
			catalogError(c, err)
			return
		}
		c.JSON(map[bool]int{true: 200, false: 201}[update], p)
	}
}
func catalogCategory(f *adminApp.CatalogAdminFacade, update bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			c.Status(401)
			return
		}
		var category catalogDomain.Category
		if err := c.ShouldBindJSON(&category); err != nil {
			c.JSON(400, gin.H{"error": "INVALID_REQUEST"})
			return
		}
		if update {
			id, err := uuid.Parse(c.Param("id"))
			if err != nil || id == uuid.Nil {
				c.JSON(400, gin.H{"error": "INVALID_ID"})
				return
			}
			category.ID = id
		}
		cmd := adminApp.CatalogCommand{ActorUserID: actor, EventKey: idempotency(c), IPAddress: c.ClientIP()}
		var err error
		if update {
			err = f.UpdateCategory(c, cmd, &category)
		} else {
			err = f.CreateCategory(c, cmd, &category)
		}
		if err != nil {
			catalogError(c, err)
			return
		}
		c.JSON(map[bool]int{true: 200, false: 201}[update], category)
	}
}
func RegisterOrdersRoutes(g *gin.RouterGroup, a adminDomain.Authorizer, f *adminApp.OrdersAdminFacade) {
	if g == nil || f == nil {
		return
	}
	g.POST("/orders/:id/cancel", RequirePermission(a, adminApp.PermissionOrdersWrite), func(c *gin.Context) {
		actor, ok := shared.AuthenticatedUserID(c)
		if !ok {
			c.Status(401)
			return
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			c.JSON(400, gin.H{"error": "INVALID_ID"})
			return
		}
		if err = f.Cancel(c, actor, idempotency(c), id, c.ClientIP()); err != nil {
			catalogError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
}
func idempotency(c *gin.Context) uuid.UUID {
	if v, err := uuid.Parse(c.GetHeader("Idempotency-Key")); err == nil && v != uuid.Nil {
		return v
	}
	return uuid.New()
}
func catalogError(c *gin.Context, err error) {
	if err == adminDomain.ErrNotAdmin || err == adminDomain.ErrPermissionDenied {
		c.JSON(403, gin.H{"error": "PERMISSION_DENIED"})
		return
	}
	c.JSON(400, gin.H{"error": "ADMIN_MUTATION_FAILED"})
}

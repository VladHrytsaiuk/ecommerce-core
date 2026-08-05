package middleware

import (
	"net/http"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

// AdminMiddleware створює gin-middleware для перевірки прав адміністратора.
// Передбачається, що цей middleware використовується ПІСЛЯ AuthMiddleware,
// який вже заповнив role_id у контексті.
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(authorizationRoleKey)
		if !exists {
			logger.Log.Warn("Role ID not found in context (AdminMiddleware)")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: no role provided"})
			return
		}

		roleCode, ok := role.(string)
		if !ok || (roleCode != "admin" && roleCode != "owner") {
			logger.Log.Warnw("User is not an admin", "role", role)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: admin access required"})
			return
		}

		c.Next()
	}
}

// OwnerMiddleware дозволяє керувати обліковими записами лише Власнику магазину.
func OwnerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(authorizationRoleKey)
		roleCode, ok := role.(string)
		if !exists || !ok || roleCode != "owner" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: owner access required"})
			return
		}
		c.Next()
	}
}

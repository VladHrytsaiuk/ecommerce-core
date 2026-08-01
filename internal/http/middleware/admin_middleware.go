package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

// AdminMiddleware створює gin-middleware для перевірки прав адміністратора.
// Передбачається, що цей middleware використовується ПІСЛЯ AuthMiddleware,
// який вже заповнив role_id у контексті.
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, exists := c.Get(authorizationRoleKey)
		if !exists {
			logger.Log.Warn("Role ID not found in context (AdminMiddleware)")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: no role provided"})
			return
		}

		roleIDInt, ok := roleID.(int)
		if !ok || !domain.IsAdminRole(roleIDInt) {
			logger.Log.Warnw("User is not an admin", "role_id", roleID)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: admin access required"})
			return
		}

		c.Next()
	}
}

// OwnerMiddleware дозволяє керувати обліковими записами лише Власнику магазину.
func OwnerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		roleID, exists := c.Get(authorizationRoleKey)
		roleIDInt, ok := roleID.(int)
		if !exists || !ok || roleIDInt != domain.RoleOwner {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: owner access required"})
			return
		}
		c.Next()
	}
}

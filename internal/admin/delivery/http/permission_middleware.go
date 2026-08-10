// Package http translates Admin authorization decisions to Gin responses.
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	sharedMiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// RequirePermission must be registered after AuthMiddleware. It evaluates a
// permission contract, never a role name or a JWT role claim.
func RequirePermission(authorizer adminDomain.Authorizer, permission string) gin.HandlerFunc {
	permission = strings.TrimSpace(permission)
	return func(c *gin.Context) {
		if authorizer == nil || permission == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "AUTHORIZATION_UNAVAILABLE"})
			return
		}
		userID, ok := sharedMiddleware.AuthenticatedUserID(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "AUTHENTICATION_REQUIRED"})
			return
		}
		err := authorizer.Require(c.Request.Context(), userID, permission)
		switch {
		case err == nil:
			c.Next()
		case errors.Is(err, adminDomain.ErrNotAdmin), errors.Is(err, adminDomain.ErrPermissionDenied), errors.Is(err, adminDomain.ErrInvalidPermission):
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "PERMISSION_DENIED"})
		default:
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "AUTHORIZATION_UNAVAILABLE"})
		}
	}
}

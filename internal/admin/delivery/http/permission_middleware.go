// Package http translates Admin authorization decisions to Gin responses.
package http

import (
	"github.com/gin-gonic/gin"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	sharedMiddleware "github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// RequirePermissionV1 evaluates a permission contract — never a role name or a
// JWT role claim — and renders its refusals as problem details.
//
// It must be registered after AuthMiddleware, since it authorizes the
// authenticated subject rather than anything in the request body.
func RequirePermissionV1(authorizer adminDomain.Authorizer, permission string, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return sharedMiddleware.RequirePermissionV1(authorizer, permission, renderer)
}

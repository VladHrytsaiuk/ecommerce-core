package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

// RequirePermissionV1 is transport infrastructure shared by all permission
// protected v1 modules. It enforces a permission contract, never a role name.
func RequirePermissionV1(authorizer adminDomain.Authorizer, permission string, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	permission = strings.TrimSpace(permission)
	return func(c *gin.Context) {
		if authorizer == nil || permission == "" {
			renderer.Abort(c, apiresponse.Unavailable(errors.New("authorization is not configured")))
			return
		}
		userID, ok := AuthenticatedUserID(c)
		if !ok {
			renderer.Abort(c, apiresponse.Unauthenticated(errors.New("authenticated subject missing")))
			return
		}
		err := authorizer.Require(c.Request.Context(), userID, permission)
		switch {
		case err == nil:
			c.Next()
		case errors.Is(err, adminDomain.ErrNotAdmin), errors.Is(err, adminDomain.ErrPermissionDenied), errors.Is(err, adminDomain.ErrInvalidPermission):
			renderer.Abort(c, apiresponse.Forbidden(err))
		default:
			renderer.Abort(c, apiresponse.Unavailable(err))
		}
	}
}

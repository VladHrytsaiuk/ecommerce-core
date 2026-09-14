package middleware

import (
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/gin-gonic/gin"
)

// OptionalAuthMiddleware authenticates when a token is present and lets the
// request through when it is not.
//
// It is for routes a guest and a signed-in customer both reach — wishlist,
// comparison, checkout — where the identity changes what is returned but its
// absence is not an error. A malformed or invalid token is treated as
// anonymous rather than refused, because the route works either way.
func OptionalAuthMiddleware(tokenMaker token.Maker) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := c.GetHeader(authorizationHeaderKey)

		// No header at all: an ordinary guest.
		if len(authorizationHeader) == 0 {
			c.Next()
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			// Malformed header: anonymous, not a 401.
			c.Next()
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			c.Next()
			return
		}

		accessToken := fields[1]
		claims, err := tokenMaker.VerifyToken(accessToken)
		if err != nil {
			logger.Log.Debugw("OptionalAuth: token verification failed, proceeding as anonymous", "error", err)
			c.Next()
			return
		}

		// Valid: publish the subject only. The role is deliberately not put in
		// the context — see AuthMiddleware.
		c.Set(authorizationPayloadKey, claims.UserID)
		c.Next()
	}
}

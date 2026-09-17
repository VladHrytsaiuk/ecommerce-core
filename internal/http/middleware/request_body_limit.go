package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

// MaxRequestBodyBytes rejects known oversized bodies before reading them and
// wraps streamed bodies with MaxBytesReader. It protects JSON binders from
// allocating attacker-controlled request sizes.
func MaxRequestBodyBytes(limit int64, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 || renderer == nil {
			if renderer != nil {
				renderer.Abort(c, apiresponse.Unavailable(errors.New("request body limit is not configured")))
			} else {
				c.AbortWithStatus(http.StatusServiceUnavailable)
			}
			return
		}
		if c.Request.ContentLength > limit {
			renderer.Abort(c, apiresponse.PayloadTooLarge(&http.MaxBytesError{Limit: limit}))
			return
		}
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		}
		c.Next()
	}
}

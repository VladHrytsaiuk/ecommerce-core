package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TimeoutMiddleware bounds how long one request may run.
//
// The deadline goes on the request context, so GORM and every adapter that
// honours cancellation stop with it rather than holding a connection for a
// client that has already gone.
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Derived from the request's own context, so a client disconnect still
		// cancels everything below.
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// Hand the bounded context to the rest of the chain.
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		// Only when nothing has been written yet: a handler that already began
		// streaming cannot be given a status code now.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{
					"error":   "Gateway Timeout",
					"message": "The request took too long to process",
				})
			}
		}
	}
}

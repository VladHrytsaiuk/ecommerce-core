package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// PanicRecovery suppresses panic values and stack traces. Versioned clients
// receive RFC 9457 INTERNAL_ERROR; legacy routes receive an empty HTTP 500.
func PanicRecovery(renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			if logger.Log != nil {
				logger.WithContext(c.Request.Context()).Errorw("HTTP handler panic recovered", "panic_type", "recovered")
			}
			if strings.HasPrefix(c.Request.URL.Path, "/api/v1/") && renderer != nil {
				renderer.Render(c, errors.New("handler panic"))
				return
			}
			c.AbortWithStatus(http.StatusInternalServerError)
		}()
		c.Next()
	}
}

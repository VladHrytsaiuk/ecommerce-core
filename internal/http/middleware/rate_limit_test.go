package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestOTPSendRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("limit exceeded", func(t *testing.T) {
		r := gin.New()
		// Limit: 1 request per 1 second
		limiter := NewOTPSendRateLimiter(1, 1*time.Second)
		r.Use(limiter.Middleware())
		r.POST("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		// First request should pass
		req1, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req1.RemoteAddr = "192.168.1.1:1234"
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)
		assert.Equal(t, http.StatusOK, w1.Code)

		// Second request should fail (Too Many Requests)
		req2, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req2.RemoteAddr = "192.168.1.1:1234"
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		assert.Equal(t, http.StatusTooManyRequests, w2.Code)

		// Request from different IP should pass
		req3, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req3.RemoteAddr = "192.168.1.2:1234"
		w3 := httptest.NewRecorder()
		r.ServeHTTP(w3, req3)
		assert.Equal(t, http.StatusOK, w3.Code)

		// Wait for interval and try again
		time.Sleep(1100 * time.Millisecond)
		req4, _ := http.NewRequest(http.MethodPost, "/test", nil)
		req4.RemoteAddr = "192.168.1.1:1234"
		w4 := httptest.NewRecorder()
		r.ServeHTTP(w4, req4)
		assert.Equal(t, http.StatusOK, w4.Code)
	})
}

package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
)

const (
	loginRateLimit       = 5
	loginRateLimitWindow = time.Minute
)

// LoginRateLimitMiddleware limits password login attempts by client IP. The
// Redis key stores an HMAC of the address, rather than the raw IP address.
func LoginRateLimitMiddleware(limiter ratelimit.Service, keySecret string) gin.HandlerFunc {
	return RateLimitByIP(limiter, "identity:login", loginRateLimit, loginRateLimitWindow, keySecret, nil)
}

// RateLimitByIP is a distributed fixed-window protection for public routes.
// With Redis enabled its limiter is shared by all API replicas; the local
// implementation remains the explicit degraded-development fallback.
func RateLimitByIP(limiter ratelimit.Service, namespace string, limit int, window time.Duration, keySecret string, renderer *apiresponse.ErrorRenderer) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "ratelimit:" + namespace + ":" + protectedClientIP(c.ClientIP(), keySecret)
		decision, err := limiter.Allow(c.Request.Context(), key, limit, window)
		if err != nil {
			// Redis is checked at boot. A later dependency failure must not silently
			// remove brute-force protection from a security-sensitive endpoint.
			if renderer != nil {
				renderer.Abort(c, apiresponse.Unavailable(err))
			} else {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "RATE_LIMIT_UNAVAILABLE"})
			}
			return
		}
		if !decision.Allowed {
			retryAfter := int(decision.RetryAfter.Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			if renderer != nil {
				renderer.Abort(c, apiresponse.RateLimited(nil))
			} else {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "RATE_LIMITED", "message": "Too many login attempts. Please try again later."})
			}
			return
		}
		c.Next()
	}
}

func protectedClientIP(ip, keySecret string) string {
	mac := hmac.New(sha256.New, []byte(keySecret))
	_, _ = mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// clientLimiter stores a rate limiter and the last time it was seen.
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// OTPSendRateLimiter manages rate limiters for multiple IP addresses.
type OTPSendRateLimiter struct {
	limiters map[string]*clientLimiter
	mu       sync.Mutex
	limit    int
	interval time.Duration
}

// NewOTPSendRateLimiter creates a new OTPSendRateLimiter with a cleanup goroutine.
func NewOTPSendRateLimiter(limit int, interval time.Duration) *OTPSendRateLimiter {
	l := &OTPSendRateLimiter{
		limiters: make(map[string]*clientLimiter),
		limit:    limit,
		interval: interval,
	}

	// Background cleanup goroutine to remove old limiters every 5 minutes.
	go l.cleanup(5 * time.Minute)

	return l
}

func (l *OTPSendRateLimiter) cleanup(interval time.Duration) {
	for {
		time.Sleep(interval)
		l.mu.Lock()
		for ip, client := range l.limiters {
			if time.Since(client.lastSeen) > 1*time.Hour {
				delete(l.limiters, ip)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware returns a Gin middleware that limits requests by IP address.
func (l *OTPSendRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()

		l.mu.Lock()
		client, exists := l.limiters[ip]
		if !exists {
			// Calculate replenishment rate (tokens per second)
			client = &clientLimiter{
				limiter: rate.NewLimiter(rate.Every(l.interval/time.Duration(l.limit)), l.limit),
			}
			l.limiters[ip] = client
		}
		client.lastSeen = time.Now()
		l.mu.Unlock()

		if !client.limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "RATE_LIMITED",
				"message": "Ви перевищили ліміт запитів. Будь ласка, спробуйте пізніше.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// visitor stores the rate limiter and the last time it was seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter is a memory-efficient rate limiter based on client IP.
type IPRateLimiter struct {
	visitors    map[string]*visitor
	mu          *sync.RWMutex
	r           rate.Limit
	b           int
	lastCleanup time.Time
}

// NewIPRateLimiter creates a new rate limiter that allows r events per second with a burst of b.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		visitors: make(map[string]*visitor),
		mu:       &sync.RWMutex{},
		r:        r,
		b:        b,
	}

	return i
}

// GetLimiter returns the rate limiter for the provided IP, creating it if it doesn't exist.
// It also updates the lastSeen timestamp.
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()
	now := time.Now()
	if i.lastCleanup.IsZero() || now.Sub(i.lastCleanup) >= time.Minute {
		for key, entry := range i.visitors {
			if now.Sub(entry.lastSeen) > 10*time.Minute {
				delete(i.visitors, key)
			}
		}
		i.lastCleanup = now
	}

	v, exists := i.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(i.r, i.b)
		i.visitors[ip] = &visitor{
			limiter:  limiter,
			lastSeen: now,
		}
		return limiter
	}

	v.lastSeen = now
	return v.limiter
}

// RateLimitMiddleware creates a Gin middleware that limits requests per IP.
func RateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "RATE_LIMITED",
				"message": "You have exceeded the allowed number of requests. Please try again later.",
			})
			return
		}
		c.Next()
	}
}

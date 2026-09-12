package middleware

import (
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// visitor stores the rate limiter and the last time it was seen.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const (
	// visitorTTL is how long an address is remembered after its last request.
	visitorTTL = 10 * time.Minute
	// visitorSweepEvery amortizes the whole-map sweep so it does not run per
	// request.
	visitorSweepEvery = time.Minute
	// maxVisitors caps the map. Age alone bounds it only by how many distinct
	// addresses appear within visitorTTL, which for IPv6 is no bound at all: a
	// client with a /64 can add an entry per request and hold it for ten
	// minutes. The cap turns that from unbounded growth into a fixed ceiling.
	maxVisitors = 50_000
	// evictFraction sets how many of the oldest entries go at once when the cap
	// is reached: a tenth of the ceiling, so the scan that finds them is
	// amortized over that many new addresses rather than run for each one. It
	// is a fraction of the limiter's own cap, not of the default, or a smaller
	// cap would empty the whole map on its first eviction.
	evictFraction = 10
)

// IPRateLimiter is a memory-efficient rate limiter based on client IP.
type IPRateLimiter struct {
	visitors    map[string]*visitor
	mu          *sync.RWMutex
	r           rate.Limit
	b           int
	lastCleanup time.Time
	maxVisitors int
}

// NewIPRateLimiter creates a new rate limiter that allows r events per second with a burst of b.
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		visitors:    make(map[string]*visitor),
		mu:          &sync.RWMutex{},
		r:           r,
		b:           b,
		maxVisitors: maxVisitors,
	}

	return i
}

// GetLimiter returns the rate limiter for the provided IP, creating it if it doesn't exist.
// It also updates the lastSeen timestamp.
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()
	now := time.Now()
	if i.lastCleanup.IsZero() || now.Sub(i.lastCleanup) >= visitorSweepEvery {
		for key, entry := range i.visitors {
			if now.Sub(entry.lastSeen) > visitorTTL {
				delete(i.visitors, key)
			}
		}
		i.lastCleanup = now
	}

	v, exists := i.visitors[ip]
	if !exists {
		if i.maxVisitors > 0 && len(i.visitors) >= i.maxVisitors {
			i.evictOldest(max(1, i.maxVisitors/evictFraction))
		}
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

// evictOldest drops the count least recently seen addresses. Their budgets
// reset, which is the right trade against unbounded memory: an address evicted
// under pressure is one that has not been heard from in longer than every other
// address currently tracked. Called only at the cap, and in batches, so the
// scan below is paid once per batch rather than once per new address.
func (i *IPRateLimiter) evictOldest(count int) {
	if count <= 0 || len(i.visitors) == 0 {
		return
	}
	type aged struct {
		key      string
		lastSeen time.Time
	}
	entries := make([]aged, 0, len(i.visitors))
	for key, entry := range i.visitors {
		entries = append(entries, aged{key: key, lastSeen: entry.lastSeen})
	}
	slices.SortFunc(entries, func(a, b aged) int { return a.lastSeen.Compare(b.lastSeen) })
	for _, entry := range entries[:min(count, len(entries))] {
		delete(i.visitors, entry.key)
	}
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

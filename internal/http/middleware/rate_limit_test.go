package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func TestRateLimitMiddlewareLimitsPerClientAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RateLimitMiddleware(NewIPRateLimiter(rate.Every(time.Hour), 1)))
	router.GET("/test", func(context *gin.Context) { context.Status(http.StatusOK) })

	if code := requestFrom(router, "192.0.2.1:1234"); code != http.StatusOK {
		t.Fatalf("first request = %d, want %d", code, http.StatusOK)
	}
	if code := requestFrom(router, "192.0.2.1:1234"); code != http.StatusTooManyRequests {
		t.Fatalf("second request from the same address = %d, want %d", code, http.StatusTooManyRequests)
	}
	// The budget is per client, so one noisy address must not lock out others.
	if code := requestFrom(router, "192.0.2.2:1234"); code != http.StatusOK {
		t.Fatalf("request from a second address = %d, want %d", code, http.StatusOK)
	}
}

func TestGetLimiterForgetsAddressesItHasNotSeenInAWhile(t *testing.T) {
	// Without this the map grows for the life of the process: one entry per
	// address ever seen, which on a public endpoint is unbounded. The cleanup
	// is amortized into GetLimiter rather than run by a goroutine, so there is
	// no background worker to leak when the limiter is discarded.
	limiter := NewIPRateLimiter(rate.Every(time.Second), 1)
	limiter.GetLimiter("192.0.2.1")
	limiter.GetLimiter("192.0.2.2")

	// Age one address past the retention window and force the next call to
	// sweep by moving the last cleanup back.
	limiter.mu.Lock()
	limiter.visitors["192.0.2.1"].lastSeen = time.Now().Add(-11 * time.Minute)
	limiter.lastCleanup = time.Now().Add(-2 * time.Minute)
	limiter.mu.Unlock()

	limiter.GetLimiter("192.0.2.3")

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if _, stale := limiter.visitors["192.0.2.1"]; stale {
		t.Fatal("an address unseen for eleven minutes is still held")
	}
	if _, recent := limiter.visitors["192.0.2.2"]; !recent {
		t.Fatal("a recently seen address was evicted; its budget would reset")
	}
}

func TestGetLimiterDoesNotSweepOnEveryCall(t *testing.T) {
	// The sweep walks the whole map, so running it per request would make the
	// limiter cost grow with the number of clients it has seen.
	limiter := NewIPRateLimiter(rate.Every(time.Second), 1)
	limiter.GetLimiter("192.0.2.1")

	limiter.mu.Lock()
	limiter.visitors["192.0.2.1"].lastSeen = time.Now().Add(-11 * time.Minute)
	firstCleanup := limiter.lastCleanup
	limiter.mu.Unlock()

	limiter.GetLimiter("192.0.2.2")

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.lastCleanup != firstCleanup {
		t.Fatal("the sweep ran again within a minute of the previous one")
	}
	if _, held := limiter.visitors["192.0.2.1"]; !held {
		t.Fatal("a stale address was evicted outside a sweep")
	}
}

func requestFrom(router *gin.Engine, address string) int {
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	request.RemoteAddr = address
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response.Code
}

package middleware

import (
	"fmt"
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

func TestTheVisitorMapHasACeiling(t *testing.T) {
	// Age-based eviction bounds the map only by how many distinct addresses
	// appear within the retention window, which for IPv6 is no bound at all: a
	// client with a /64 can add an entry per request and hold it ten minutes.
	limiter := NewIPRateLimiter(rate.Every(time.Second), 1)
	limiter.maxVisitors = 100

	for n := range 1000 {
		limiter.GetLimiter(fmt.Sprintf("2001:db8::%x", n))
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.visitors) > limiter.maxVisitors {
		t.Fatalf("the map holds %d addresses, want at most %d", len(limiter.visitors), limiter.maxVisitors)
	}
	if len(limiter.visitors) == 0 {
		t.Fatal("the map was emptied; every client would get a fresh budget")
	}
}

func TestTheMostRecentlySeenAddressesSurviveEviction(t *testing.T) {
	// Whoever is evicted gets their budget back, so it has to be whoever has
	// been quiet longest — not an active client mid-request.
	limiter := NewIPRateLimiter(rate.Every(time.Second), 1)
	limiter.maxVisitors = 10

	for n := range 10 {
		limiter.GetLimiter(fmt.Sprintf("198.51.100.%d", n))
	}
	// Keep one address warm, then push the map past its cap.
	limiter.GetLimiter("198.51.100.9")
	for n := range 5 {
		limiter.GetLimiter(fmt.Sprintf("203.0.113.%d", n))
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if _, kept := limiter.visitors["198.51.100.9"]; !kept {
		t.Fatal("the most recently seen address was evicted ahead of quieter ones")
	}
	if _, kept := limiter.visitors["198.51.100.0"]; kept {
		t.Fatal("the least recently seen address survived while newer ones were added")
	}
}

func TestEvictionLeavesAnActiveClientLimited(t *testing.T) {
	// The point of the ceiling is memory, not amnesty. An address that is
	// actively being limited must not get a fresh bucket because someone else
	// rotated addresses.
	limiter := NewIPRateLimiter(rate.Every(time.Hour), 1)
	limiter.maxVisitors = 50

	const active = "198.51.100.200"
	if !limiter.GetLimiter(active).Allow() {
		t.Fatal("the first request was refused")
	}
	for n := range 200 {
		limiter.GetLimiter(fmt.Sprintf("2001:db8:1::%x", n))
		// The active client keeps being heard from throughout the flood.
		limiter.GetLimiter(active)
	}
	if limiter.GetLimiter(active).Allow() {
		t.Fatal("an actively limited address got a fresh budget during an address flood")
	}
}

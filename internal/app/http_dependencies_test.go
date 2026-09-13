package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
)

// Login and public browsing share one rate-limit middleware, and an error from
// the limiter refuses the request. That is right for login — losing
// brute-force protection is worse than failing a login — and wrong for a
// catalog page, where it turned a Redis blip into 503 for the whole versioned
// API. The split is expressed by which limiter each is handed, so it is worth
// checking that composition survives.

func TestARedisOutageDoesNotTakeTheStorefrontDown(t *testing.T) {
	dependencies := dependenciesWithFailingLimiter(t)

	if code := serveThrough(dependencies, dependencies.APIRateLimit); code != http.StatusOK {
		t.Fatalf("public browsing returned %d with the limiter store unreachable, want %d", code, http.StatusOK)
	}
}

func TestARedisOutageStillRefusesLogin(t *testing.T) {
	dependencies := dependenciesWithFailingLimiter(t)

	if code := serveThrough(dependencies, dependencies.LoginRateLimit); code == http.StatusOK {
		t.Fatal("login was allowed with no working brute-force protection")
	}
}

func dependenciesWithFailingLimiter(t *testing.T) HTTPDependencies {
	t.Helper()
	gin.SetMode(gin.TestMode)
	maker, err := token.NewJWTMaker("test-secret-that-is-long-enough-for-jwt")
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := buildHTTPDependencies(
		&config.Config{JWTSecret: "test-secret-that-is-long-enough-for-jwt", APIRateLimitPerMin: 100, RequestTimeout: time.Minute, CORSAllowOrigins: []string{"https://store.example"}},
		StoreConfig{DefaultLocale: "en", FallbackLocale: "en", SupportedLocales: []string{"en"}},
		maker,
		unreachableLimiter{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return dependencies
}

// serveThrough builds the chain the real router builds. The error renderer has
// to be in it: the limiter reports a refusal by aborting with an error for that
// middleware to render, so without it an abort produces gin's default 200 and
// the assertion below passes whatever the limiter decided.
func serveThrough(dependencies HTTPDependencies, middleware gin.HandlerFunc) int {
	router := gin.New()
	router.Use(dependencies.ErrorRenderer.Middleware(), middleware)
	router.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
	return recorder.Code
}

// unreachableLimiter stands in for Redis being down.
type unreachableLimiter struct{}

func (unreachableLimiter) Allow(context.Context, string, int, time.Duration) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, errors.New("dial tcp: connection refused")
}

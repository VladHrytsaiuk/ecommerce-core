package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/middleware"
)

// Handlers throughout this codebase hand *gin.Context straight to application
// services, which is the shape Gin's own examples use. A *gin.Context only
// behaves like the request context it wraps when the engine opts in: without
// ContextWithFallback it reports no deadline, never cancels, and cannot read a
// value stored under a non-string key. TimeoutMiddleware and the observability
// middleware both work by replacing the request context, so all three of these
// pin behaviour the rest of the system assumes.

type probeKey struct{}

func TestTheGinContextCarriesTheRequestDeadline(t *testing.T) {
	var deadline time.Time
	var ok bool
	probe(t, 50*time.Millisecond, func(c *gin.Context) { deadline, ok = c.Deadline() })

	if !ok {
		t.Fatal("a service given the Gin context sees no deadline: TimeoutMiddleware cannot interrupt its queries")
	}
	if until := time.Until(deadline); until <= 0 || until > time.Minute {
		t.Fatalf("deadline is %s away, want the request timeout", until)
	}
}

func TestTheGinContextCancelsWithTheRequest(t *testing.T) {
	var done <-chan struct{}
	var err error
	probe(t, 10*time.Millisecond, func(c *gin.Context) {
		done = c.Done()
		time.Sleep(40 * time.Millisecond) // outlives the timeout set above
		err = c.Err()
	})

	if done == nil {
		t.Fatal("a service given the Gin context gets a nil Done channel: nothing downstream can be cancelled")
	}
	if err == nil {
		t.Fatal("the Gin context reports no error after the request context expired")
	}
}

func TestTheGinContextSeesValuesFromTheRequestContext(t *testing.T) {
	// Tracing spans and the request ID are stored under unexported key types,
	// which never match Gin's own string-keyed store.
	var value any
	probe(t, time.Minute, func(c *gin.Context) { value = c.Value(probeKey{}) })

	if value != "carried" {
		t.Fatalf("c.Value() = %v, want the request-context value; spans and request IDs are stored this way", value)
	}
}

func TestTrustedProxiesAreRejectedRatherThanIgnored(t *testing.T) {
	// Falling back to Gin's defaults here would let a forged X-Forwarded-For
	// choose the client IP that rate limiting and audit logging record.
	defer func() {
		if recover() == nil {
			t.Fatal("newEngine accepted an invalid trusted proxy")
		}
	}()
	newEngine([]string{"not-a-cidr"})
}

// probe runs one request through an engine built the way InitRouter builds it,
// behind the real timeout middleware.
func probe(t *testing.T, timeout time.Duration, handler gin.HandlerFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := newEngine(nil)
	router.Use(middleware.TimeoutMiddleware(timeout), func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), probeKey{}, "carried"))
	})
	router.GET("/probe", handler)
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))
}

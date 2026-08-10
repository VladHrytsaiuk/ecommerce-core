package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/ratelimit"
)

func TestLoginRateLimitMiddlewareRejectsSixthAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/auth/login", LoginRateLimitMiddleware(ratelimit.NewLocalService(), "test-secret"), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for attempt := 1; attempt <= 6; attempt++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		request.RemoteAddr = "198.51.100.42:1234"
		router.ServeHTTP(recorder, request)
		want := http.StatusNoContent
		if attempt == 6 {
			want = http.StatusTooManyRequests
			if recorder.Header().Get("Retry-After") == "" {
				t.Fatal("sixth attempt must include Retry-After")
			}
		}
		if recorder.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt, recorder.Code, want)
		}
	}
}

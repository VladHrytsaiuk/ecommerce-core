package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

func TestMiddlewarePropagatesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.Init()
	router := gin.New()
	router.Use(Middleware())
	router.GET("/orders/:id", func(c *gin.Context) {
		if got := logger.RequestID(c.Request.Context()); got != "request-123" {
			t.Fatalf("request ID in context = %q", got)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/orders/42", nil)
	request.Header.Set("X-Request-ID", "request-123")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("X-Request-ID"); got != "request-123" {
		t.Fatalf("response request ID = %q", got)
	}
}

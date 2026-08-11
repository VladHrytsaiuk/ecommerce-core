package apiresponse

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func TestSuccessIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/resource", func(c *gin.Context) { Success(c, http.StatusOK, gin.H{"id": "product-1"}) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	request = request.WithContext(logger.WithRequestID(request.Context(), "request-1"))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"request_id":"request-1"`) {
		t.Fatalf("response misses request ID: %s", recorder.Body.String())
	}
}

func TestErrorRendererDoesNotExposeOriginalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	renderer := NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware())
	router.GET("/resource", func(c *gin.Context) {
		renderer.Abort(c, InvalidPayload(errors.New("postgres password=do-not-expose")))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	request = request.WithContext(logger.WithRequestID(request.Context(), "request-2"))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("content type = %q", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"code":"INVALID_PAYLOAD"`) || !strings.Contains(body, `"request_id":"request-2"`) {
		t.Fatalf("unexpected problem response: %s", body)
	}
	if strings.Contains(body, "postgres password") {
		t.Fatalf("original error leaked to client: %s", body)
	}
}

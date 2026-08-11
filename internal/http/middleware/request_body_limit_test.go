package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

func TestMaxRequestBodyBytesRejectsKnownOversizedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	renderer := apiresponse.NewErrorRenderer(nil)
	router := gin.New()
	router.Use(renderer.Middleware(), MaxRequestBodyBytes(16, renderer))
	router.POST("/resource", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/resource", strings.NewReader(strings.Repeat("x", 17))))

	if recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), `"code":"PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("oversized body = status %d, body %s", recorder.Code, recorder.Body.String())
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
)

func TestPanicRecoveryReturnsMaskedProblemForV1(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(PanicRecovery(apiresponse.NewErrorRenderer(nil)))
	router.GET("/api/v1/panic", func(*gin.Context) { panic("postgres://user:password@host") })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil))

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) || strings.Contains(recorder.Body.String(), "password") {
		t.Fatalf("panic response leaks or uses wrong contract: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

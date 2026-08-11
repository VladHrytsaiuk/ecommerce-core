package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsConfiguredCredentialedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	policy, err := NewCORS([]string{"https://store.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(policy)
	router.OPTIONS("/api/v1/catalog/uk/products", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/catalog/uk/products", nil)
	request.Header.Set("Origin", "https://store.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://store.example.com" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q", got)
	}
	if got := recorder.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary header = %q, want it to include Origin", got)
	}
}

func TestCORSDoesNotReflectUnknownOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	policy, err := NewCORS([]string{"https://store.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(policy)
	router.GET("/resource", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	request.Header.Set("Origin", "https://evil.example")
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected reflected origin %q", got)
	}
}

func TestCORSRejectsWildcardOrigin(t *testing.T) {
	if _, err := NewCORS([]string{"*"}); err == nil {
		t.Fatal("expected wildcard origin to be rejected")
	}
}

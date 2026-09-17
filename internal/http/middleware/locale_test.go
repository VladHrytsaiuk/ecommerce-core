package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewLocaleMiddlewareUsesConfiguredLocalesAndFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/:lang", NewLocaleMiddleware(LocaleOptions{
		DefaultLocale:    "es",
		FallbackLocale:   "es",
		SupportedLocales: []string{"es", "en", "ca"},
	}), func(c *gin.Context) {
		c.String(http.StatusOK, GetLanguage(c))
	})

	tests := []struct {
		path string
		want string
	}{
		{path: "/ca", want: "ca"},
		{path: "/EN", want: "en"},
		{path: "/uk", want: "es"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(response, request)

			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, tt.want, response.Body.String())
		})
	}
}

func TestLocaleMiddlewarePreservesUkrainianAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/:lang", NewLocaleMiddleware(LocaleOptions{
		DefaultLocale:    "uk",
		FallbackLocale:   "uk",
		SupportedLocales: []string{"uk", "en"},
	}), func(c *gin.Context) {
		c.String(http.StatusOK, GetLanguage(c))
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ua", nil))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "uk", response.Body.String())
}

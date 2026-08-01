package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

type mockLogger struct{}

func (m *mockLogger) Debug(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Info(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Warn(msg string, fields ...zap.Field)        {}
func (m *mockLogger) Error(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Fatal(msg string, fields ...zap.Field)       {}
func (m *mockLogger) Debugf(template string, args ...interface{}) {}
func (m *mockLogger) Infof(template string, args ...interface{})  {}
func (m *mockLogger) Warnf(template string, args ...interface{})  {}
func (m *mockLogger) Errorf(template string, args ...interface{}) {}
func (m *mockLogger) Fatalf(template string, args ...interface{}) {}
func (m *mockLogger) Debugw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Infow(msg string, kvs ...interface{})        {}
func (m *mockLogger) Warnw(msg string, kvs ...interface{})        {}
func (m *mockLogger) Errorw(msg string, kvs ...interface{})       {}
func (m *mockLogger) Fatalw(msg string, kvs ...interface{})       {}
func (m *mockLogger) With(fields ...zap.Field) logger.Logger        { return m }
func (m *mockLogger) Sync() error                                 { return nil }

type mockSitemapCache struct {
	index      []byte
	products   []byte
	categories []byte
	brands     []byte
	documents  []byte
}

func (m *mockSitemapCache) GetIndex() []byte      { return m.index }
func (m *mockSitemapCache) GetProducts() []byte   { return m.products }
func (m *mockSitemapCache) GetCategories() []byte { return m.categories }
func (m *mockSitemapCache) GetBrands() []byte     { return m.brands }
func (m *mockSitemapCache) GetDocuments() []byte  { return m.documents }

func TestSitemapHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &mockSitemapCache{
		index:      []byte("<sitemapindex></sitemapindex>"),
		products:   []byte("<urlset><url><loc>products</loc></url></urlset>"),
		categories: []byte("<urlset><url><loc>categories</loc></url></urlset>"),
		brands:     []byte("<urlset><url><loc>brands</loc></url></urlset>"),
		documents:  []byte("<urlset><url><loc>documents</loc></url></urlset>"),
	}

	cfg := &config.Config{
		FrontendURL: "https://example.com",
	}

	handler := NewSitemapHandler(cache, cfg, &mockLogger{})
	r := gin.New()
	RegisterSitemapRoutes(r.Group("/"), handler)

	tests := []struct {
		name     string
		url      string
		expected string
		status   int
	}{
		{"Index", "/sitemap.xml", "<sitemapindex></sitemapindex>", http.StatusOK},
		{"Index_Alias", "/sitemaps/index.xml", "<sitemapindex></sitemapindex>", http.StatusOK},
		{"Products", "/sitemaps/products.xml", "<urlset><url><loc>products</loc></url></urlset>", http.StatusOK},
		{"Categories", "/sitemaps/categories.xml", "<urlset><url><loc>categories</loc></url></urlset>", http.StatusOK},
		{"Brands", "/sitemaps/brands.xml", "<urlset><url><loc>brands</loc></url></urlset>", http.StatusOK},
		{"Documents", "/sitemaps/documents.xml", "<urlset><url><loc>documents</loc></url></urlset>", http.StatusOK},
		{"RobotsTxt", "/robots.txt", "Sitemap: https://example.com/api/sitemap.xml", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tc.url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.status, w.Code)
			assert.Contains(t, w.Body.String(), tc.expected)
			if tc.url != "/robots.txt" {
				assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestSitemapHandler_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Empty cache means not generated yet
	cache := &mockSitemapCache{}
	cfg := &config.Config{}
	handler := NewSitemapHandler(cache, cfg, &mockLogger{})
	r := gin.New()
	RegisterSitemapRoutes(r.Group("/"), handler)

	tests := []string{
		"/sitemap.xml",
		"/sitemaps/index.xml",
		"/sitemaps/products.xml",
		"/sitemaps/categories.xml",
		"/sitemaps/brands.xml",
		"/sitemaps/documents.xml",
	}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code)
			assert.Contains(t, w.Body.String(), "not found or not yet generated")
		})
	}
}

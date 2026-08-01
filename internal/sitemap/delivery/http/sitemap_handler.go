package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/domain"
)

type SitemapHandler struct {
	cache domain.SitemapCache
	cfg   *config.Config
	l     logger.Logger
}

func NewSitemapHandler(cache domain.SitemapCache, cfg *config.Config, l logger.Logger) *SitemapHandler {
	return &SitemapHandler{
		cache: cache,
		cfg:   cfg,
		l:     l,
	}
}

func RegisterSitemapRoutes(r *gin.RouterGroup, h *SitemapHandler) {
	sitemaps := r.Group("/sitemaps")
	{
		sitemaps.GET("/index.xml", h.GetSitemapIndex)
		sitemaps.GET("/products.xml", h.GetProductsSitemap)
		sitemaps.GET("/categories.xml", h.GetCategoriesSitemap)
		sitemaps.GET("/brands.xml", h.GetBrandsSitemap)
		sitemaps.GET("/documents.xml", h.GetDocumentsSitemap)
	}
	
	// Зворотна сумісність: кореневий sitemap.xml часто шукають в корені сайту,
	// але бекенд зазвичай за nginx. У нашому випадку API віддасть за /api/sitemap.xml
	r.GET("/sitemap.xml", h.GetSitemapIndex)
	
	// robots.txt
	r.GET("/robots.txt", h.GetRobotsTxt)
}

// GetSitemapIndex returns the main sitemap index
func (h *SitemapHandler) GetSitemapIndex(c *gin.Context) {
	data := h.cache.GetIndex()
	if len(data) == 0 {
		c.String(http.StatusNotFound, "Sitemap not found or not yet generated")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

// GetProductsSitemap returns products sitemap
func (h *SitemapHandler) GetProductsSitemap(c *gin.Context) {
	data := h.cache.GetProducts()
	if len(data) == 0 {
		c.String(http.StatusNotFound, "Sitemap not found or not yet generated")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

// GetCategoriesSitemap returns categories sitemap
func (h *SitemapHandler) GetCategoriesSitemap(c *gin.Context) {
	data := h.cache.GetCategories()
	if len(data) == 0 {
		c.String(http.StatusNotFound, "Sitemap not found or not yet generated")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

// GetBrandsSitemap returns brands sitemap
func (h *SitemapHandler) GetBrandsSitemap(c *gin.Context) {
	data := h.cache.GetBrands()
	if len(data) == 0 {
		c.String(http.StatusNotFound, "Sitemap not found or not yet generated")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

// GetDocumentsSitemap returns documents sitemap
func (h *SitemapHandler) GetDocumentsSitemap(c *gin.Context) {
	data := h.cache.GetDocuments()
	if len(data) == 0 {
		c.String(http.StatusNotFound, "Sitemap not found or not yet generated")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

// GetRobotsTxt returns dynamic robots.txt file
func (h *SitemapHandler) GetRobotsTxt(c *gin.Context) {
	robotsTxt := "User-agent: *\n" +
		"Disallow: /cart\n" +
		"Disallow: /checkout\n" +
		"Disallow: /admin\n" +
		"Disallow: /search\n" +
		"Disallow: /*?sort_by=\n" +
		"Disallow: /*?order=\n" +
		"Disallow: /*?brand_id=\n" +
		"Disallow: /*?min_price=\n" +
		"Disallow: /*?max_price=\n" +
		"Disallow: /*?quantity_values=\n" +
		"Disallow: /*?attrs[\n" +
		"Disallow: /*?q=\n\n" +
		"Sitemap: " + h.cfg.FrontendURL + "/api/sitemap.xml\n"

	c.String(http.StatusOK, robotsTxt)
}

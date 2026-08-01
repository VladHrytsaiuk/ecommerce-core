package service

import (
	"context"
	"encoding/xml"
	"strings"
	"sync/atomic"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/domain"
	"go.uber.org/zap"
)

type cacheStore struct {
	index      []byte
	products   []byte
	categories []byte
	brands     []byte
	documents  []byte
}

type sitemapWorker struct {
	repo   domain.SitemapRepository
	logger logger.Logger
	cfg    *config.Config
	cache  atomic.Value // holds *cacheStore
}

func NewSitemapWorker(repo domain.SitemapRepository, cfg *config.Config, l logger.Logger) domain.SitemapWorkerService {
	w := &sitemapWorker{
		repo:   repo,
		logger: l,
		cfg:    cfg,
	}
	// Init empty cache
	w.cache.Store(&cacheStore{})
	return w
}

func (w *sitemapWorker) GetIndex() []byte      { return w.cache.Load().(*cacheStore).index }
func (w *sitemapWorker) GetProducts() []byte   { return w.cache.Load().(*cacheStore).products }
func (w *sitemapWorker) GetCategories() []byte { return w.cache.Load().(*cacheStore).categories }
func (w *sitemapWorker) GetBrands() []byte     { return w.cache.Load().(*cacheStore).brands }
func (w *sitemapWorker) GetDocuments() []byte  { return w.cache.Load().(*cacheStore).documents }

// Run executes sitemap regeneration until ctx is cancelled.
func (w *sitemapWorker) Run(ctx context.Context, interval time.Duration) {
	w.logger.Info("Starting Sitemap Worker", zap.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// First run immediately
	if err := w.GenerateAll(ctx); err != nil {
		w.logger.Error("Failed initial sitemap generation", zap.Error(err))
	}

	for {
		select {
		case <-ticker.C:
			if err := w.GenerateAll(ctx); err != nil {
				w.logger.Error("Failed to generate sitemap", zap.Error(err))
			}
		case <-ctx.Done():
			w.logger.Info("Sitemap Worker stopped")
			return
		}
	}
}

func (w *sitemapWorker) GenerateAll(ctx context.Context) error {
	w.logger.Info("Generating sitemaps...")
	startTime := time.Now()

	baseURL := w.cfg.FrontendURL
	if baseURL == "" {
		baseURL = "https://aquawheel.store" // fallback
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	// 1. Fetch data
	products, err := w.repo.GetActiveProductSlugs(ctx)
	if err != nil {
		return err
	}
	categories, err := w.repo.GetCategorySlugs(ctx)
	if err != nil {
		return err
	}
	brands, err := w.repo.GetBrandSlugs(ctx)
	if err != nil {
		return err
	}
	docs, err := w.repo.GetDocumentSlugs(ctx)
	if err != nil {
		return err
	}

	nowStr := time.Now().Format("2006-01-02")

	// For brands: duplicate them into UK and EN entries since brands have a single global slug
	var localizedBrands []domain.EntityInfo
	for _, b := range brands {
		localizedBrands = append(localizedBrands, domain.EntityInfo{
			Slug:         b.Slug,
			LanguageCode: "uk",
			UpdatedAt:    b.UpdatedAt,
		})
		localizedBrands = append(localizedBrands, domain.EntityInfo{
			Slug:         b.Slug,
			LanguageCode: "en",
			UpdatedAt:    b.UpdatedAt,
		})
	}

	// For documents: duplicate them into UK and EN entries since documents have a single global slug
	var localizedDocs []domain.EntityInfo
	for _, d := range docs {
		localizedDocs = append(localizedDocs, domain.EntityInfo{
			Slug:         d.Slug,
			LanguageCode: "uk",
			UpdatedAt:    d.UpdatedAt,
		})
		localizedDocs = append(localizedDocs, domain.EntityInfo{
			Slug:         d.Slug,
			LanguageCode: "en",
			UpdatedAt:    d.UpdatedAt,
		})
	}

	// 2. Build individual sitemaps
	productsXML, err := buildURLSet(baseURL, "/product/", products, "weekly", 0.7)
	if err != nil {
		return err
	}
	categoriesXML, err := buildURLSet(baseURL, "/category/", categories, "daily", 0.9)
	if err != nil {
		return err
	}
	brandsXML, err := buildURLSet(baseURL, "/brand/", localizedBrands, "weekly", 0.8)
	if err != nil {
		return err
	}
	docsXML, err := buildURLSet(baseURL, "/pages/", localizedDocs, "monthly", 0.5)
	if err != nil {
		return err
	}

	// 3. Build Sitemap Index (excluding /api/ path for routing transparency)
	index := domain.SitemapIndex{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		Sitemaps: []domain.SitemapEntry{
			{Loc: baseURL + "/sitemaps/categories.xml", LastMod: nowStr},
			{Loc: baseURL + "/sitemaps/brands.xml", LastMod: nowStr},
			{Loc: baseURL + "/sitemaps/products.xml", LastMod: nowStr},
			{Loc: baseURL + "/sitemaps/documents.xml", LastMod: nowStr},
		},
	}

	indexBytes, err := xml.Marshal(index)
	if err != nil {
		return err
	}
	indexBytes = append([]byte(xml.Header), indexBytes...)

	// 4. Update atomic cache safely
	newCache := &cacheStore{
		index:      indexBytes,
		products:   productsXML,
		categories: categoriesXML,
		brands:     brandsXML,
		documents:  docsXML,
	}
	w.cache.Store(newCache)

	w.logger.Info("Sitemaps generated successfully", zap.Duration("took", time.Since(startTime)))
	return nil
}

func buildURLSet(baseURL string, path string, items []domain.EntityInfo, freq string, priority float64) ([]byte, error) {
	set := domain.URLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  make([]domain.URL, 0, len(items)),
	}

	for _, item := range items {
		loc := baseURL
		if item.LanguageCode == "en" {
			loc += "/en"
		}
		loc += path + item.Slug

		set.URLs = append(set.URLs, domain.URL{
			Loc:        loc,
			LastMod:    item.UpdatedAt.Format("2006-01-02"),
			ChangeFreq: freq,
			Priority:   priority,
		})
	}

	data, err := xml.Marshal(set)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), data...), nil
}

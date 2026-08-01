package domain

import (
	"context"
	"encoding/xml"
	"time"
)

// URL представляє один запис у sitemap.xml
type URL struct {
	XMLName    xml.Name `xml:"url"`
	Loc        string   `xml:"loc"`
	LastMod    string   `xml:"lastmod"`
	ChangeFreq string   `xml:"changefreq,omitempty"`
	Priority   float64  `xml:"priority,omitempty"`
}

// URLSet кореневий елемент для sitemap.xml (якщо це не індекс)
type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	Xmlns   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

// SitemapEntry елемент у sitemapindex
type SitemapEntry struct {
	XMLName xml.Name `xml:"sitemap"`
	Loc     string   `xml:"loc"`
	LastMod string   `xml:"lastmod,omitempty"`
}

// SitemapIndex кореневий елемент для головного sitemap.xml
type SitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Xmlns    string         `xml:"xmlns,attr"`
	Sitemaps []SitemapEntry `xml:"sitemap"`
}

// EntityInfo скорочена інформація про сутність для Sitemap
type EntityInfo struct {
	Slug         string
	LanguageCode string
	UpdatedAt    time.Time
}

// SitemapRepository визначає методи для швидкого доступу до необхідних для Sitemap даних
type SitemapRepository interface {
	GetActiveProductSlugs(ctx context.Context) ([]EntityInfo, error)
	GetCategorySlugs(ctx context.Context) ([]EntityInfo, error)
	GetBrandSlugs(ctx context.Context) ([]EntityInfo, error)
	GetDocumentSlugs(ctx context.Context) ([]EntityInfo, error)
}

// SitemapCache визначає інтерфейс для доступу до закешованих XML
type SitemapCache interface {
	GetIndex() []byte
	GetProducts() []byte
	GetCategories() []byte
	GetBrands() []byte
	GetDocuments() []byte
}

// SitemapWorkerService об'єднує генерацію та кешування
type SitemapWorkerService interface {
	SitemapCache
	Start(ctx context.Context, interval time.Duration)
	Run(ctx context.Context, interval time.Duration)
	GenerateAll(ctx context.Context) error
}

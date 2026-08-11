// Package domain defines the provider-neutral Search module contracts.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSnapshotNotFound = errors.New("catalog search snapshot not found")
	ErrUnavailable      = errors.New("search provider unavailable")
	ErrInvalidQuery     = errors.New("invalid search query")
)

// SearchDocument is a product-level read projection. Its optional commerce
// facets are deliberately provider-neutral and will be enriched by the
// Catalog/Inventory snapshot adapter as those attributes become available.
type SearchDocument struct {
	ID           string              `json:"id"`
	Locales      []string            `json:"locales"`
	Names        []string            `json:"names"`
	Descriptions []string            `json:"descriptions"`
	Slugs        []string            `json:"slugs"`
	CategoryID   string              `json:"category_id,omitempty"`
	Status       string              `json:"status"`
	Brand        string              `json:"brand,omitempty"`
	Attributes   map[string][]string `json:"attributes,omitempty"`
	PriceMin     *int64              `json:"price_min,omitempty"`
	PriceMax     *int64              `json:"price_max,omitempty"`
	Currency     string              `json:"currency,omitempty"`
	InStock      *bool               `json:"in_stock,omitempty"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// CatalogSnapshotProvider intentionally exposes a search DTO instead of a
// Catalog aggregate, keeping the Search domain independent of Catalog storage.
type CatalogSnapshotProvider interface {
	Snapshot(context.Context, uuid.UUID) (*SearchDocument, error)
}

// DocumentIndexer is implemented by search-engine adapters. Calls only submit
// a document task; no external I/O occurs inside the database transaction.
type DocumentIndexer interface {
	AddOrUpdateDocuments(context.Context, ...SearchDocument) error
	DeleteDocument(context.Context, uuid.UUID) error
}

type Filters struct {
	Brands     []string
	PriceMin   *int64
	PriceMax   *int64
	InStock    *bool
	Attributes map[string][]string
}

// ProductSearchQuery is normalized and validated by the application layer.
// Filter contains only DSL emitted by the allow-listed filter builder.
type ProductSearchQuery struct {
	Query  string
	Locale string
	Page   int
	Limit  int
	Sort   []string
	Filter string
}

type Facets map[string]map[string]int64

type ProductSearchResult struct {
	Documents []SearchDocument
	Total     int64
	Facets    Facets
}

type Suggestion struct {
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
}

type ProductSearchEngine interface {
	Search(context.Context, ProductSearchQuery) (ProductSearchResult, error)
	Autocomplete(context.Context, ProductSearchQuery) ([]Suggestion, error)
}

type SearchService interface {
	SearchProducts(context.Context, SearchProductsInput) (ProductSearchResult, error)
	Autocomplete(context.Context, AutocompleteInput) ([]Suggestion, error)
}

type SearchProductsInput struct {
	Query   string
	Locale  string
	Page    int
	Limit   int
	Sort    string
	Filters Filters
}

type AutocompleteInput struct {
	Query  string
	Locale string
	Limit  int
}

// ActiveDocumentLister is intentionally separate from point snapshots: only
// the local maintenance reindex command needs broad catalog enumeration.
type ActiveDocumentLister interface {
	ListActiveDocuments(context.Context, *uuid.UUID, int) ([]SearchDocument, *uuid.UUID, error)
}

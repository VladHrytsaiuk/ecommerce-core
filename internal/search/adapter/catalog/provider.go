// Package catalog adapts the public Catalog product port to Search snapshots.
package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	catalogDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	search "github.com/VladHrytsaiuk/ecommerce-core/internal/search/domain"
)

type ProductReader interface {
	FindByID(context.Context, uuid.UUID) (*catalogDomain.Product, error)
	ListActiveAfter(context.Context, *uuid.UUID, int) ([]catalogDomain.Product, error)
}

type SnapshotProvider struct{ products ProductReader }

func NewSnapshotProvider(products ProductReader) *SnapshotProvider {
	return &SnapshotProvider{products: products}
}

func (p *SnapshotProvider) Snapshot(ctx context.Context, productID uuid.UUID) (*search.SearchDocument, error) {
	if p == nil || p.products == nil || productID == uuid.Nil {
		return nil, search.ErrSnapshotNotFound
	}
	product, err := p.products.FindByID(ctx, productID)
	if errors.Is(err, catalogDomain.ErrProductNotFound) {
		return nil, search.ErrSnapshotNotFound
	}
	if err != nil || product == nil {
		return nil, err
	}
	return documentFromProduct(product), nil
}

func (p *SnapshotProvider) ListActiveDocuments(ctx context.Context, after *uuid.UUID, limit int) ([]search.SearchDocument, *uuid.UUID, error) {
	if p == nil || p.products == nil || limit < 1 {
		return nil, nil, search.ErrSnapshotNotFound
	}
	products, err := p.products.ListActiveAfter(ctx, after, limit)
	if err != nil {
		return nil, nil, err
	}
	documents := make([]search.SearchDocument, 0, len(products))
	for index := range products {
		documents = append(documents, *documentFromProduct(&products[index]))
	}
	if len(products) == 0 {
		return documents, nil, nil
	}
	next := products[len(products)-1].ID
	return documents, &next, nil
}

func documentFromProduct(product *catalogDomain.Product) *search.SearchDocument {
	document := &search.SearchDocument{
		ID:           product.ID.String(),
		Locales:      make([]string, 0, len(product.Translations)),
		Names:        make([]string, 0, len(product.Translations)),
		Descriptions: make([]string, 0, len(product.Translations)),
		Slugs:        make([]string, 0, len(product.Translations)),
		Status:       strings.TrimSpace(product.Status),
		UpdatedAt:    product.UpdatedAt.UTC(),
	}
	if product.CategoryID != nil {
		document.CategoryID = product.CategoryID.String()
	}
	for _, translation := range product.Translations {
		document.Locales = append(document.Locales, translation.Locale)
		document.Names = append(document.Names, translation.Name)
		document.Descriptions = append(document.Descriptions, translation.Description)
		document.Slugs = append(document.Slugs, translation.Slug)
	}
	return document
}

var _ search.CatalogSnapshotProvider = (*SnapshotProvider)(nil)
var _ search.ActiveDocumentLister = (*SnapshotProvider)(nil)

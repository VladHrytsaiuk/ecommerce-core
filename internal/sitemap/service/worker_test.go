package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/sitemap/domain"
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
func (m *mockLogger) With(fields ...zap.Field) logger.Logger      { return m }
func (m *mockLogger) Sync() error                                 { return nil }

type mockSitemapRepo struct {
	products   []domain.EntityInfo
	categories []domain.EntityInfo
	brands     []domain.EntityInfo
	docs       []domain.EntityInfo
}

func (m *mockSitemapRepo) GetActiveProductSlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	return m.products, nil
}
func (m *mockSitemapRepo) GetCategorySlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	return m.categories, nil
}
func (m *mockSitemapRepo) GetBrandSlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	return m.brands, nil
}
func (m *mockSitemapRepo) GetDocumentSlugs(ctx context.Context) ([]domain.EntityInfo, error) {
	return m.docs, nil
}

func TestSitemapWorker(t *testing.T) {
	repo := &mockSitemapRepo{
		products: []domain.EntityInfo{
			{Slug: "product-1", LanguageCode: "uk", UpdatedAt: time.Now()},
		},
		categories: []domain.EntityInfo{
			{Slug: "cat-1", LanguageCode: "en", UpdatedAt: time.Now()},
		},
		brands: []domain.EntityInfo{
			{Slug: "brand-1", UpdatedAt: time.Now()}, // Will be duplicated to uk and en
		},
		docs: []domain.EntityInfo{
			{Slug: "doc-1", UpdatedAt: time.Now()}, // Will be duplicated to uk and en
		},
	}

	cfg := &config.Config{
		FrontendURL: "https://example.com",
	}

	worker := NewSitemapWorker(repo, cfg, &mockLogger{})
	ctx := context.Background()

	err := worker.GenerateAll(ctx)
	assert.NoError(t, err)

	index := string(worker.GetIndex())
	products := string(worker.GetProducts())
	categories := string(worker.GetCategories())
	brands := string(worker.GetBrands())
	docs := string(worker.GetDocuments())

	assert.True(t, strings.Contains(index, "sitemapindex"))
	assert.True(t, strings.Contains(index, "categories.xml"))

	assert.True(t, strings.Contains(products, "product-1"))
	assert.True(t, strings.Contains(products, "https://example.com/product/product-1"))

	assert.True(t, strings.Contains(categories, "cat-1"))
	assert.True(t, strings.Contains(categories, "https://example.com/en/category/cat-1"))

	assert.True(t, strings.Contains(brands, "brand-1"))
	assert.True(t, strings.Contains(brands, "https://example.com/brand/brand-1"))
	assert.True(t, strings.Contains(brands, "https://example.com/en/brand/brand-1"))

	assert.True(t, strings.Contains(docs, "doc-1"))
	assert.True(t, strings.Contains(docs, "https://example.com/pages/doc-1"))
	assert.True(t, strings.Contains(docs, "https://example.com/en/pages/doc-1"))
}

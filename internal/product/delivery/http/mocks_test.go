package http

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"go.uber.org/zap"
)

// ==========================================
// MockProductService
// ==========================================

type MockProductService struct {
	mock.Mock
}

func (m *MockProductService) GetList(ctx context.Context, lang string, filter domain.ProductFilter, pgn pagination.Params) ([]domain.ProductVariation, pagination.Metadata, error) {
	args := m.Called(ctx, lang, filter, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(pagination.Metadata), args.Error(2)
	}
	return args.Get(0).([]domain.ProductVariation), args.Get(1).(pagination.Metadata), args.Error(2)
}

func (m *MockProductService) GetRecommended(ctx context.Context, lang string, limit int) ([]domain.ProductVariation, error) {
	args := m.Called(ctx, lang, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ProductVariation), args.Error(1)
}

func (m *MockProductService) QuickSearch(ctx context.Context, query string, lang string, limit int) ([]domain.QuickSearchProduct, error) {
	args := m.Called(ctx, query, lang, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.QuickSearchProduct), args.Error(1)
}

func (m *MockProductService) GetByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Product, error) {
	args := m.Called(ctx, id, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductService) GetBySlug(ctx context.Context, slug string, lang string) (*domain.Product, error) {
	args := m.Called(ctx, slug, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductService) GenerateSlug(ctx context.Context, name string, excludeID uuid.UUID) (string, error) {
	args := m.Called(ctx, name, excludeID)
	return args.String(0), args.Error(1)
}

func (m *MockProductService) GetReviews(ctx context.Context, productID uuid.UUID, pgn pagination.Params) ([]domain.ProductReview, pagination.Metadata, error) {
	args := m.Called(ctx, productID, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(pagination.Metadata), args.Error(2)
	}
	return args.Get(0).([]domain.ProductReview), args.Get(1).(pagination.Metadata), args.Error(2)
}

func (m *MockProductService) GetPendingReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, pagination.Metadata, error) {
	args := m.Called(ctx, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(pagination.Metadata), args.Error(2)
	}
	return args.Get(0).([]domain.ProductReview), args.Get(1).(pagination.Metadata), args.Error(2)
}

func (m *MockProductService) GetRejectedReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, pagination.Metadata, error) {
	args := m.Called(ctx, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(pagination.Metadata), args.Error(2)
	}
	return args.Get(0).([]domain.ProductReview), args.Get(1).(pagination.Metadata), args.Error(2)
}

func (m *MockProductService) AddReview(ctx context.Context, review *domain.ProductReview) error {
	return m.Called(ctx, review).Error(0)
}

func (m *MockProductService) ApproveReview(ctx context.Context, reviewID uuid.UUID) error {
	args := m.Called(ctx, reviewID)
	return args.Error(0)
}

func (m *MockProductService) RejectReview(ctx context.Context, reviewID uuid.UUID, reason *string) error {
	args := m.Called(ctx, reviewID, reason)
	return args.Error(0)
}

func (m *MockProductService) ReopenReview(ctx context.Context, reviewID uuid.UUID) error {
	return m.Called(ctx, reviewID).Error(0)
}

func (m *MockProductService) GetFilters(ctx context.Context, filter domain.ProductFilter, lang string) (*domain.FilterDiscovery, error) {
	args := m.Called(ctx, filter, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.FilterDiscovery), args.Error(1)
}

func (m *MockProductService) GetUnits(ctx context.Context) ([]domain.Unit, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Unit), args.Error(1)
}

func (m *MockProductService) CreateProduct(ctx context.Context, product *domain.Product, images []domain.ImageUpload) error {
	return m.Called(ctx, product, images).Error(0)
}

func (m *MockProductService) UpdateProduct(ctx context.Context, product *domain.Product, variationsUpdate *[]domain.ProductVariationUpdate, newImages []domain.ImageUpload, imagesToDelete []uuid.UUID, isActive *bool, isBundle *bool, attributeValuesProvided bool) error {
	return m.Called(ctx, product, variationsUpdate, newImages, imagesToDelete, isActive, isBundle, attributeValuesProvided).Error(0)
}

func (m *MockProductService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockProductService) UploadProductImages(ctx context.Context, productID uuid.UUID, images []domain.ImageUpload) ([]domain.ProductImage, error) {
	args := m.Called(ctx, productID, images)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ProductImage), args.Error(1)
}

func (m *MockProductService) DeleteProductImage(ctx context.Context, productID uuid.UUID, imageID uuid.UUID) error {
	return m.Called(ctx, productID, imageID).Error(0)
}

func (m *MockProductService) UpdateProductImage(ctx context.Context, productID uuid.UUID, imageID uuid.UUID, isPrimary, isHover *bool, sortOrder *int, variationIDSet *bool, variationID *uuid.UUID, altTextUk, altTextEn *string) error {
	return m.Called(ctx, productID, imageID, isPrimary, isHover, sortOrder, variationIDSet, variationID, altTextUk, altTextEn).Error(0)
}

func (m *MockProductService) ReorderProductImages(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) error {
	return m.Called(ctx, productID, ids).Error(0)
}

func (m *MockProductService) UploadMedia(ctx context.Context, file interface{}, folder, filename string) (string, error) {
	args := m.Called(ctx, file, folder, filename)
	return args.String(0), args.Error(1)
}

// ==========================================
// MockBrandService
// ==========================================

type MockBrandService struct {
	mock.Mock
}

func (m *MockBrandService) GetAll(ctx context.Context) ([]domain.Brand, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Brand), args.Error(1)
}

func (m *MockBrandService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Brand), args.Error(1)
}

func (m *MockBrandService) Create(ctx context.Context, brand *domain.Brand) error {
	return m.Called(ctx, brand).Error(0)
}

func (m *MockBrandService) Update(ctx context.Context, brand *domain.Brand) error {
	return m.Called(ctx, brand).Error(0)
}

func (m *MockBrandService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockBrandService) GenerateSlug(ctx context.Context, name string) (string, error) {
	args := m.Called(ctx, name)
	return args.String(0), args.Error(1)
}

// ==========================================
// noopLogger — беззвучний логер для тестів
// ==========================================

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Info(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Warn(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Error(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Fatal(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Debugf(template string, args ...interface{}) {}
func (n *noopLogger) Infof(template string, args ...interface{})  {}
func (n *noopLogger) Warnf(template string, args ...interface{})  {}
func (n *noopLogger) Errorf(template string, args ...interface{}) {}
func (n *noopLogger) Fatalf(template string, args ...interface{}) {}
func (n *noopLogger) Debugw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Infow(msg string, kvs ...interface{})        {}
func (n *noopLogger) Warnw(msg string, kvs ...interface{})        {}
func (n *noopLogger) Errorw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Fatalw(msg string, kvs ...interface{})       {}
func (n *noopLogger) With(fields ...zap.Field) logger.Logger      { return n }
func (n *noopLogger) Sync() error                                 { return nil }

// ==========================================
// Helpers
// ==========================================

func toJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return bytes.NewBuffer(data)
}

// ==========================================
// MockBadgeService
// ==========================================

type MockBadgeService struct {
	mock.Mock
}

func (m *MockBadgeService) GetAll(ctx context.Context) ([]domain.Badge, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Badge), args.Error(1)
}

func (m *MockBadgeService) GetByID(ctx context.Context, id int) (*domain.Badge, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Badge), args.Error(1)
}

func (m *MockBadgeService) Create(ctx context.Context, badge *domain.Badge) error {
	return m.Called(ctx, badge).Error(0)
}

func (m *MockBadgeService) Update(ctx context.Context, badge *domain.Badge) error {
	return m.Called(ctx, badge).Error(0)
}

func (m *MockBadgeService) Delete(ctx context.Context, id int) error {
	return m.Called(ctx, id).Error(0)
}

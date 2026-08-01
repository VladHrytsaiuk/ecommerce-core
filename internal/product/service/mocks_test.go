package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"go.uber.org/zap"
)

// ==========================================
// MockProductRepository
// ==========================================

type MockProductRepository struct {
	mock.Mock
}

func (m *MockProductRepository) FindAll(ctx context.Context, lang string, filter domain.ProductFilter, pgn pagination.Params) ([]domain.ProductVariation, int64, error) {
	args := m.Called(ctx, lang, filter, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]domain.ProductVariation), args.Get(1).(int64), args.Error(2)
}

func (m *MockProductRepository) FindRecommended(ctx context.Context, lang string, limit int) ([]domain.ProductVariation, error) {
	args := m.Called(ctx, lang, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ProductVariation), args.Error(1)
}

func (m *MockProductRepository) QuickSearch(ctx context.Context, query string, lang string, limit int) ([]domain.QuickSearchProduct, error) {
	args := m.Called(ctx, query, lang, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.QuickSearchProduct), args.Error(1)
}

func (m *MockProductRepository) FindByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Product, error) {
	args := m.Called(ctx, id, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductRepository) FindBySlug(ctx context.Context, slug string, lang string) (*domain.Product, error) {
	args := m.Called(ctx, slug, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Product), args.Error(1)
}

func (m *MockProductRepository) SlugExists(ctx context.Context, slug string, excludeID uuid.UUID) (bool, error) {
	args := m.Called(ctx, slug, excludeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockProductRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockProductRepository) FindReviews(ctx context.Context, productID uuid.UUID, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	args := m.Called(ctx, productID, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]domain.ProductReview), args.Get(1).(int64), args.Error(2)
}

func (m *MockProductRepository) FindPendingReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	args := m.Called(ctx, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]domain.ProductReview), args.Get(1).(int64), args.Error(2)
}

func (m *MockProductRepository) FindRejectedReviews(ctx context.Context, pgn pagination.Params) ([]domain.ProductReview, int64, error) {
	args := m.Called(ctx, pgn)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]domain.ProductReview), args.Get(1).(int64), args.Error(2)
}

func (m *MockProductRepository) CreateReview(ctx context.Context, review *domain.ProductReview) error {
	return m.Called(ctx, review).Error(0)
}

func (m *MockProductRepository) ApproveReview(ctx context.Context, reviewID uuid.UUID) error {
	args := m.Called(ctx, reviewID)
	return args.Error(0)
}

func (m *MockProductRepository) RejectReview(ctx context.Context, reviewID uuid.UUID, reason *string) error {
	args := m.Called(ctx, reviewID, reason)
	return args.Error(0)
}

func (m *MockProductRepository) ReopenReview(ctx context.Context, reviewID uuid.UUID) error {
	return m.Called(ctx, reviewID).Error(0)
}

func (m *MockProductRepository) GetFilters(ctx context.Context, filter domain.ProductFilter, lang string) (*domain.FilterDiscovery, error) {
	args := m.Called(ctx, filter, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.FilterDiscovery), args.Error(1)
}

func (m *MockProductRepository) GetUnits(ctx context.Context) ([]domain.Unit, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Unit), args.Error(1)
}

func (m *MockProductRepository) Create(ctx context.Context, product *domain.Product) error {
	return m.Called(ctx, product).Error(0)
}

func (m *MockProductRepository) Update(ctx context.Context, product *domain.Product) error {
	return m.Called(ctx, product).Error(0)
}

func (m *MockProductRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockProductRepository) CountByBrand(ctx context.Context, brandID uuid.UUID) (int64, error) {
	args := m.Called(ctx, brandID)
	return int64(args.Int(0)), args.Error(1)
}

func (m *MockProductRepository) CountByCategory(ctx context.Context, categoryID uuid.UUID) (int64, error) {
	args := m.Called(ctx, categoryID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockProductRepository) GetActiveCategoryIDs(ctx context.Context) (map[uuid.UUID]bool, error) {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(map[uuid.UUID]bool), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockProductRepository) AreVariationsNonBundle(ctx context.Context, variationIDs []uuid.UUID) (bool, error) {
	args := m.Called(ctx, variationIDs)
	return args.Bool(0), args.Error(1)
}

func (m *MockProductRepository) FindVariationsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.ProductVariation, error) {
	args := m.Called(ctx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ProductVariation), args.Error(1)
}

func (m *MockProductRepository) FindAttributeByCode(ctx context.Context, code string) (*domain.Attribute, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Attribute), args.Error(1)
}

func (m *MockProductRepository) FindImageByID(ctx context.Context, imageID uuid.UUID) (*domain.ProductImage, error) {
	args := m.Called(ctx, imageID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ProductImage), args.Error(1)
}

func (m *MockProductRepository) DeleteImage(ctx context.Context, imageID uuid.UUID) error {
	return m.Called(ctx, imageID).Error(0)
}

func (m *MockProductRepository) UpdateImage(ctx context.Context, image *domain.ProductImage) error {
	return m.Called(ctx, image).Error(0)
}

func (m *MockProductRepository) ReorderImages(ctx context.Context, productID uuid.UUID, ids []uuid.UUID) error {
	return m.Called(ctx, productID, ids).Error(0)
}

func (m *MockProductRepository) CreateImages(ctx context.Context, images []domain.ProductImage) error {
	return m.Called(ctx, images).Error(0)
}

func (m *MockProductRepository) ResetImageRole(ctx context.Context, productID uuid.UUID, variationID *uuid.UUID, field string) error {
	return m.Called(ctx, productID, variationID, field).Error(0)
}

// ==========================================
// MockBrandRepository
// ==========================================

type MockBrandRepository struct {
	mock.Mock
}

func (m *MockBrandRepository) FindAll(ctx context.Context) ([]domain.Brand, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Brand), args.Error(1)
}

func (m *MockBrandRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Brand), args.Error(1)
}

func (m *MockBrandRepository) Create(ctx context.Context, brand *domain.Brand) error {
	return m.Called(ctx, brand).Error(0)
}

func (m *MockBrandRepository) Update(ctx context.Context, brand *domain.Brand) error {
	return m.Called(ctx, brand).Error(0)
}

func (m *MockBrandRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockBrandRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	args := m.Called(ctx, slug)
	return args.Bool(0), args.Error(1)
}

// ==========================================
// MockStorage
// ==========================================

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) Upload(ctx context.Context, file interface{}, folder, filename string) (string, error) {
	args := m.Called(ctx, file, folder, filename)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) Delete(ctx context.Context, publicID string) error {
	return m.Called(ctx, publicID).Error(0)
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
// MockBadgeRepository
// ==========================================

type MockBadgeRepository struct {
	mock.Mock
}

func (m *MockBadgeRepository) FindAll(ctx context.Context) ([]domain.Badge, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Badge), args.Error(1)
}

func (m *MockBadgeRepository) FindByID(ctx context.Context, id int) (*domain.Badge, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Badge), args.Error(1)
}

func (m *MockBadgeRepository) Create(ctx context.Context, badge *domain.Badge) error {
	return m.Called(ctx, badge).Error(0)
}

func (m *MockBadgeRepository) Update(ctx context.Context, badge *domain.Badge) error {
	return m.Called(ctx, badge).Error(0)
}

func (m *MockBadgeRepository) Delete(ctx context.Context, id int) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockBadgeRepository) CountUsage(ctx context.Context, badgeID int) (int64, error) {
	args := m.Called(ctx, badgeID)
	return args.Get(0).(int64), args.Error(1)
}

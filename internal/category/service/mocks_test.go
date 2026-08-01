package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

// ==========================================
// MockProductChecker
// ==========================================

type MockProductChecker struct {
	mock.Mock
}

func (m *MockProductChecker) CountByCategory(ctx context.Context, categoryID uuid.UUID) (int64, error) {
	args := m.Called(ctx, categoryID)
	return int64(args.Int(0)), args.Error(1)
}

func (m *MockProductChecker) GetActiveCategoryIDs(ctx context.Context) (map[uuid.UUID]bool, error) {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(map[uuid.UUID]bool), args.Error(1)
	}
	return nil, args.Error(1)
}

// ==========================================
// MockCategoryRepository
// ==========================================

type MockCategoryRepository struct {
	mock.Mock
}

func (m *MockCategoryRepository) FindAll(ctx context.Context, lang string) ([]domain.Category, error) {
	args := m.Called(ctx, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Category), args.Error(1)
}

func (m *MockCategoryRepository) FindByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Category, error) {
	args := m.Called(ctx, id, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Category), args.Error(1)
}

func (m *MockCategoryRepository) FindBySlug(ctx context.Context, slug string, lang string) (*domain.Category, error) {
	args := m.Called(ctx, slug, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Category), args.Error(1)
}

func (m *MockCategoryRepository) Create(ctx context.Context, category *domain.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}

func (m *MockCategoryRepository) Update(ctx context.Context, category *domain.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}

func (m *MockCategoryRepository) SlugExists(ctx context.Context, slug string, excludeID uuid.UUID) (bool, error) {
	args := m.Called(ctx, slug, excludeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockCategoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockCategoryRepository) UpdateOrder(ctx context.Context, ids []uuid.UUID) error {
	args := m.Called(ctx, ids)
	return args.Error(0)
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

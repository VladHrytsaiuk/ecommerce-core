package http

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"go.uber.org/zap"
)

// ==========================================
// MockCategoryService
// ==========================================

type MockCategoryService struct {
	mock.Mock
}

func (m *MockCategoryService) GetList(ctx context.Context, lang string, showAll bool) ([]domain.Category, error) {
	args := m.Called(ctx, lang, showAll)
	if args.Get(0) != nil {
		return args.Get(0).([]domain.Category), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockCategoryService) GetByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Category, error) {
	args := m.Called(ctx, id, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Category), args.Error(1)
}

func (m *MockCategoryService) GetBySlug(ctx context.Context, slug string, lang string) (*domain.Category, error) {
	args := m.Called(ctx, slug, lang)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Category), args.Error(1)
}

func (m *MockCategoryService) Create(ctx context.Context, category *domain.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}

func (m *MockCategoryService) GenerateSlug(ctx context.Context, name string, excludeID uuid.UUID) (string, error) {
	args := m.Called(ctx, name, excludeID)
	return args.String(0), args.Error(1)
}

func (m *MockCategoryService) Update(ctx context.Context, category *domain.Category) error {
	args := m.Called(ctx, category)
	return args.Error(0)
}

func (m *MockCategoryService) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockCategoryService) UpdateOrder(ctx context.Context, ids []uuid.UUID) error {
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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
	"go.uber.org/zap"
)

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

type mockProvider struct {
	mock.Mock
}

func (m *mockProvider) GetAreas(ctx context.Context) ([]domain.Area, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Area), args.Error(1)
}

func (m *mockProvider) GetCities(ctx context.Context, areaRef string) ([]domain.City, error) {
	args := m.Called(ctx, areaRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.City), args.Error(1)
}

func (m *mockProvider) GetWarehouses(ctx context.Context, cityRef string, warehouseType string) ([]domain.Warehouse, error) {
	args := m.Called(ctx, cityRef, warehouseType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Warehouse), args.Error(1)
}

type mockRuleRepo struct {
	mock.Mock
}

func (m *mockRuleRepo) GetActiveRules(ctx context.Context) ([]domain.ShippingRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ShippingRule), args.Error(1)
}

func (m *mockRuleRepo) UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error {
	args := m.Called(ctx, provider, minOrderAmount)
	return args.Error(0)
}

func TestShipmentService_GetAreas(t *testing.T) {
	mockProv := new(mockProvider)
	mockRepo := new(mockRuleRepo)
	providers := map[string]domain.ShipmentProvider{
		"test_prov": mockProv,
	}

	s := NewShipmentService(providers, mockRepo, &noopLogger{})
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expectedAreas := []domain.Area{{Ref: "1", Description: "Area1"}}
		mockProv.On("GetAreas", ctx).Return(expectedAreas, nil).Once()

		areas, err := s.GetAreas(ctx, "test_prov")
		assert.NoError(t, err)
		assert.Equal(t, expectedAreas, areas)
		mockProv.AssertExpectations(t)
	})

	t.Run("unknown provider", func(t *testing.T) {
		_, err := s.GetAreas(ctx, "unknown")
		assert.ErrorIs(t, err, domain.ErrUnknownProvider)
	})

	t.Run("provider error", func(t *testing.T) {
		expectedErr := errors.New("api error")
		mockProv.On("GetAreas", ctx).Return(nil, expectedErr).Once()

		_, err := s.GetAreas(ctx, "test_prov")
		assert.ErrorIs(t, err, expectedErr)
		mockProv.AssertExpectations(t)
	})
}

func TestShipmentService_GetCities(t *testing.T) {
	mockProv := new(mockProvider)
	mockRepo := new(mockRuleRepo)
	providers := map[string]domain.ShipmentProvider{
		"test_prov": mockProv,
	}
	s := NewShipmentService(providers, mockRepo, &noopLogger{})
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expectedCities := []domain.City{{Ref: "c1", Description: "City1", AreaRef: "a1"}}
		mockProv.On("GetCities", ctx, "a1").Return(expectedCities, nil).Once()

		cities, err := s.GetCities(ctx, "test_prov", "a1")
		assert.NoError(t, err)
		assert.Equal(t, expectedCities, cities)
		mockProv.AssertExpectations(t)
	})

	t.Run("unknown provider", func(t *testing.T) {
		_, err := s.GetCities(ctx, "unknown", "a1")
		assert.ErrorIs(t, err, domain.ErrUnknownProvider)
	})
}

func TestShipmentService_GetWarehouses(t *testing.T) {
	mockProv := new(mockProvider)
	mockRepo := new(mockRuleRepo)
	providers := map[string]domain.ShipmentProvider{
		"test_prov": mockProv,
	}
	s := NewShipmentService(providers, mockRepo, &noopLogger{})
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expectedWh := []domain.Warehouse{{Ref: "w1", Description: "Branch1"}}
		mockProv.On("GetWarehouses", ctx, "c1", "branch").Return(expectedWh, nil).Once()

		wh, err := s.GetWarehouses(ctx, "test_prov", "c1", "branch")
		assert.NoError(t, err)
		assert.Equal(t, expectedWh, wh)
		mockProv.AssertExpectations(t)
	})

	t.Run("unknown provider", func(t *testing.T) {
		_, err := s.GetWarehouses(ctx, "unknown", "c1", "branch")
		assert.ErrorIs(t, err, domain.ErrUnknownProvider)
	})
}

func TestShipmentService_GetFreeShippingThreshold(t *testing.T) {
	mockRepo := new(mockRuleRepo)
	s := NewShipmentService(nil, mockRepo, &noopLogger{})
	ctx := context.Background()

	t.Run("found global rule", func(t *testing.T) {
		rules := []domain.ShippingRule{
			{Provider: "all", MinOrderAmount: 60000},
		}
		mockRepo.On("GetActiveRules", ctx).Return(rules, nil).Once()

		threshold, err := s.GetFreeShippingThreshold(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 60000, threshold)
	})

	t.Run("no rules found", func(t *testing.T) {
		mockRepo.On("GetActiveRules", ctx).Return([]domain.ShippingRule{}, nil).Once()

		threshold, err := s.GetFreeShippingThreshold(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, threshold)
	})
}

func TestShipmentService_GetMinimumOrderAmount(t *testing.T) {
	mockRepo := new(mockRuleRepo)
	s := NewShipmentService(nil, mockRepo, &noopLogger{})
	ctx := context.Background()

	t.Run("found min order rule", func(t *testing.T) {
		rules := []domain.ShippingRule{
			{Provider: "min_order", MinOrderAmount: 20000},
		}
		mockRepo.On("GetActiveRules", ctx).Return(rules, nil).Once()

		amount, err := s.GetMinimumOrderAmount(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 20000, amount)
	})

	t.Run("no rules found", func(t *testing.T) {
		mockRepo.On("GetActiveRules", ctx).Return([]domain.ShippingRule{}, nil).Once()

		amount, err := s.GetMinimumOrderAmount(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, amount)
	})
}

func TestShipmentService_UpdateShippingRule(t *testing.T) {
	mockRepo := new(mockRuleRepo)
	s := NewShipmentService(nil, mockRepo, &noopLogger{})
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		mockRepo.On("UpdateShippingRule", ctx, "min_order", 20000).Return(nil).Once()

		err := s.UpdateShippingRule(ctx, "min_order", 20000)
		assert.NoError(t, err)
	})
}

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

type mockService struct {
	mock.Mock
}

func (m *mockService) GetAreas(ctx context.Context, provider string) ([]domain.Area, error) {
	args := m.Called(ctx, provider)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Area), args.Error(1)
}

func (m *mockService) GetCities(ctx context.Context, provider string, areaRef string) ([]domain.City, error) {
	args := m.Called(ctx, provider, areaRef)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.City), args.Error(1)
}

func (m *mockService) GetWarehouses(ctx context.Context, provider string, cityRef string, warehouseType string) ([]domain.Warehouse, error) {
	args := m.Called(ctx, provider, cityRef, warehouseType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Warehouse), args.Error(1)
}

func (m *mockService) GetFreeShippingThreshold(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *mockService) GetMinimumOrderAmount(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *mockService) UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error {
	args := m.Called(ctx, provider, minOrderAmount)
	return args.Error(0)
}

func setupRouter(svc *mockService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	h := NewShipmentHandler(svc, &noopLogger{})

	r.GET("/api/shipment/:provider/areas", h.GetAreas)
	r.GET("/api/shipment/:provider/cities", h.GetCities)
	r.GET("/api/shipment/:provider/warehouses", h.GetWarehouses)

	return r
}

func TestShipmentHandler_GetAreas(t *testing.T) {
	mockSvc := new(mockService)
	r := setupRouter(mockSvc)

	t.Run("success", func(t *testing.T) {
		expectedAreas := []domain.Area{{Ref: "1", Description: "Area1"}}
		mockSvc.On("GetAreas", mock.Anything, "novaposhta").Return(expectedAreas, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/novaposhta/areas", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []AreaResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response, 1)
		assert.Equal(t, "1", response[0].Ref)
	})

	t.Run("unknown provider", func(t *testing.T) {
		mockSvc.On("GetAreas", mock.Anything, "unknown").Return(nil, domain.ErrUnknownProvider).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/unknown/areas", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("service error", func(t *testing.T) {
		mockSvc.On("GetAreas", mock.Anything, "novaposhta").Return(nil, errors.New("timeout")).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/novaposhta/areas", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}

func TestShipmentHandler_GetCities(t *testing.T) {
	mockSvc := new(mockService)
	r := setupRouter(mockSvc)

	t.Run("success", func(t *testing.T) {
		expectedCities := []domain.City{{Ref: "c1", Description: "City1", AreaRef: "a1"}}
		mockSvc.On("GetCities", mock.Anything, "novaposhta", "a1").Return(expectedCities, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/novaposhta/cities?area_ref=a1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []CityResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response, 1)
		assert.Equal(t, "c1", response[0].Ref)
	})

	t.Run("missing area_ref", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/novaposhta/cities", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown provider", func(t *testing.T) {
		mockSvc.On("GetCities", mock.Anything, "unknown", "a1").Return(nil, domain.ErrUnknownProvider).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/unknown/cities?area_ref=a1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestShipmentHandler_GetWarehouses(t *testing.T) {
	mockSvc := new(mockService)
	r := setupRouter(mockSvc)

	t.Run("success", func(t *testing.T) {
		expectedWh := []domain.Warehouse{{Ref: "w1", Description: "Wh1", TypeOfWarehouse: "Branch"}}
		mockSvc.On("GetWarehouses", mock.Anything, "novaposhta", "c1", "branch").Return(expectedWh, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/novaposhta/warehouses?city_ref=c1&type=branch", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []WarehouseResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response, 1)
		assert.Equal(t, "w1", response[0].Ref)
	})

	t.Run("missing city_ref", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/shipment/novaposhta/warehouses?type=branch", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

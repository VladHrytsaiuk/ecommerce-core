//go:build legacy
// +build legacy

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
)

type MockManagerService struct {
	mock.Mock
	domain.ManagerService
}

func (m *MockManagerService) GetOrderByToken(ctx context.Context, orderNumber int64, plainToken string) (*domain.Order, error) {
	args := m.Called(ctx, orderNumber, plainToken)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockManagerService) ConfirmOrder(ctx context.Context, orderNumber int64, plainToken string) (*domain.Order, error) {
	args := m.Called(ctx, orderNumber, plainToken)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockManagerService) CancelOrder(ctx context.Context, orderNumber int64, plainToken string) error {
	args := m.Called(ctx, orderNumber, plainToken)
	return args.Error(0)
}

func setupManagerTestRouter() (*gin.Engine, *MockManagerService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	mockSvc := new(MockManagerService)
	handler := NewManagerHandler(mockSvc, &noopLogger{})

	// Add routes with typical manager paths
	router.GET("/manager/orders/:orderNumber", handler.GetManagerOrder)
	router.POST("/manager/orders/:orderNumber/confirm", handler.ConfirmOrder)
	router.POST("/manager/orders/:orderNumber/cancel", handler.CancelOrder)

	return router, mockSvc
}

func TestGetManagerOrder(t *testing.T) {
	router, mockSvc := setupManagerTestRouter()

	order := &domain.Order{
		ID:          uuid.New(),
		OrderNumber: 1001,
		Status: domain.OrderStatus{
			ID:   domain.StatusPaid,
			Code: "paid",
		},
	}

	mockSvc.On("GetOrderByToken", mock.Anything, int64(1001), "secret").Return(order, nil)

	req, _ := http.NewRequest(http.MethodGet, "/manager/orders/1001?token=secret", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestManagerConfirmOrder(t *testing.T) {
	router, mockSvc := setupManagerTestRouter()

	order := &domain.Order{
		ID:          uuid.New(),
		OrderNumber: 1001,
		TTNNumber:   "TTN123",
	}

	mockSvc.On("ConfirmOrder", mock.Anything, int64(1001), "secret").Return(order, nil)

	req, _ := http.NewRequest(http.MethodPost, "/manager/orders/1001/confirm?token=secret", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var res map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, "TTN123", res["ttn_number"])
}

func TestManagerCancelOrder(t *testing.T) {
	router, mockSvc := setupManagerTestRouter()

	mockSvc.On("CancelOrder", mock.Anything, int64(1001), "secret").Return(nil)

	req, _ := http.NewRequest(http.MethodPost, "/manager/orders/1001/cancel?token=secret", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

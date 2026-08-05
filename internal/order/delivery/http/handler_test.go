//go:build legacy && ignore
// +build legacy,ignore

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
	"go.uber.org/zap"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

type noopLogger struct{}

func (l *noopLogger) Debug(msg string, fields ...zap.Field) {}
func (l *noopLogger) Info(msg string, fields ...zap.Field)  {}
func (l *noopLogger) Warn(msg string, fields ...zap.Field)  {}
func (l *noopLogger) Error(msg string, fields ...zap.Field) {}
func (l *noopLogger) Fatal(msg string, fields ...zap.Field) {}

func (l *noopLogger) Debugf(template string, args ...interface{}) {}
func (l *noopLogger) Infof(template string, args ...interface{})  {}
func (l *noopLogger) Warnf(template string, args ...interface{})  {}
func (l *noopLogger) Errorf(template string, args ...interface{}) {}
func (l *noopLogger) Fatalf(template string, args ...interface{}) {}

func (l *noopLogger) Debugw(msg string, kvs ...interface{}) {}
func (l *noopLogger) Infow(msg string, kvs ...interface{})  {}
func (l *noopLogger) Warnw(msg string, kvs ...interface{})  {}
func (l *noopLogger) Errorw(msg string, kvs ...interface{}) {}
func (l *noopLogger) Fatalw(msg string, kvs ...interface{}) {}

func (l *noopLogger) Sync() error                            { return nil }
func (l *noopLogger) With(fields ...zap.Field) logger.Logger { return l }

type MockOrderService struct {
	mock.Mock
	domain.OrderService
}

func (m *MockOrderService) CreateOrder(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string, input domain.CreateOrderInput) (*domain.CreateOrderResult, error) {
	args := m.Called(ctx, userID, sessionID, lang, input)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.CreateOrderResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockOrderService) GetByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockOrderService) GetOrderStatusByID(ctx context.Context, orderID uuid.UUID) (int, error) {
	args := m.Called(ctx, orderID)
	return args.Int(0), args.Error(1)
}

func (m *MockOrderService) GetMyOrders(ctx context.Context, userID uuid.UUID, pgn pagination.Params) ([]domain.Order, pagination.Metadata, error) {
	args := m.Called(ctx, userID, pgn)
	if args.Get(0) != nil {
		return args.Get(0).([]domain.Order), args.Get(1).(pagination.Metadata), args.Error(2)
	}
	return nil, pagination.Metadata{}, args.Error(2)
}

func (m *MockOrderService) CancelOrderByUser(ctx context.Context, userID uuid.UUID, orderID uuid.UUID) error {
	args := m.Called(ctx, userID, orderID)
	return args.Error(0)
}

func (m *MockOrderService) GeneratePaymentURL(ctx context.Context, orderID uuid.UUID) (string, error) {
	args := m.Called(ctx, orderID)
	return args.String(0), args.Error(1)
}

func setupTestRouter() (*gin.Engine, *MockOrderService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	mockSvc := new(MockOrderService)
	handler := NewOrderHandler(mockSvc, &noopLogger{})

	// Inject dummy user_id for test
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-User-ID") != "" {
			c.Set("user_id", c.GetHeader("X-User-ID"))
			c.Set("role_id", 1) // role customer
		}
		if c.GetHeader("X-Session-ID") != "" {
			c.Set("session_id", c.GetHeader("X-Session-ID"))
		}
		c.Set("lang", "en")
		c.Next()
	})

	router.POST("/orders", handler.CreateOrder)
	router.GET("/orders/:id", handler.GetOrder)
	router.GET("/orders/:id/payment-status", handler.GetPaymentStatus)
	router.GET("/my-orders", handler.GetMyOrders)
	router.POST("/orders/:id/cancel", handler.CancelOrder)

	return router, mockSvc
}

func TestGetOrder(t *testing.T) {
	router, mockSvc := setupTestRouter()
	orderID := uuid.New()
	userID := uuid.New()

	order := &domain.Order{
		ID:       orderID,
		UserID:   &userID,
		StatusID: domain.StatusPendingPayment,
		Status: domain.OrderStatus{
			ID:   domain.StatusPendingPayment,
			Code: "pending_payment",
		},
	}

	mockSvc.On("GetByID", mock.Anything, orderID).Return(order, nil)
	mockSvc.On("GeneratePaymentURL", mock.Anything, orderID).Return("http://pay.me", nil)

	req, _ := http.NewRequest(http.MethodGet, "/orders/"+orderID.String(), nil)
	req.Header.Set("X-User-ID", userID.String())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var res OrderResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, orderID, res.ID)
	assert.Equal(t, "http://pay.me", res.PaymentURL)
}

func TestCancelOrder(t *testing.T) {
	router, mockSvc := setupTestRouter()
	orderID := uuid.New()
	userID := uuid.New()

	mockSvc.On("CancelOrderByUser", mock.Anything, userID, orderID).Return(nil)

	req, _ := http.NewRequest(http.MethodPost, "/orders/"+orderID.String()+"/cancel", nil)
	req.Header.Set("X-User-ID", userID.String())
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

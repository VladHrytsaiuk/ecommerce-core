package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	paymentHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

type mockPaymentService struct {
	mock.Mock
}

func (m *mockPaymentService) CreatePayment(ctx context.Context, orderID uuid.UUID, amount int) error {
	args := m.Called(ctx, orderID, amount)
	return args.Error(0)
}

func (m *mockPaymentService) ProcessWebhook(ctx context.Context, data, signature string) error {
	args := m.Called(ctx, data, signature)
	return args.Error(0)
}

func (m *mockPaymentService) GeneratePaymentURL(orderID uuid.UUID, amount int, orderNumber int64, paytypes string) string {
	args := m.Called(orderID, amount, orderNumber, paytypes)
	return args.String(0)
}

func (m *mockPaymentService) SimulatePayment(ctx context.Context, orderID uuid.UUID) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *mockPaymentService) ProcessRefundStub(ctx context.Context, orderID uuid.UUID) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func setupTestRouter(svc *mockPaymentService) *gin.Engine {
	logger.Init()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := paymentHttp.NewPaymentHandler(svc, logger.Log)
	r.POST("/api/webhooks/liqpay", handler.HandleLiqPayWebhook)
	r.POST("/api/webhooks/mock-payment", handler.HandleMockPayment)
	return r
}

func TestPaymentHandler_HandleLiqPayWebhook(t *testing.T) {
	svc := new(mockPaymentService)
	r := setupTestRouter(svc)

	t.Run("Success", func(t *testing.T) {
		svc.ExpectedCalls = nil // reset mocks
		svc.On("ProcessWebhook", mock.Anything, "valid_data", "valid_sig").Return(nil).Once()

		data := url.Values{}
		data.Set("data", "valid_data")
		data.Set("signature", "valid_sig")

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/liqpay", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
		svc.AssertExpectations(t)
	})

	t.Run("InvalidBody", func(t *testing.T) {
		svc.ExpectedCalls = nil
		
		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/liqpay", strings.NewReader("invalid_body"))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("ProcessWebhook", mock.Anything, "invalid_data", "invalid_sig").Return(domain.ErrInvalidSignature).Once()

		data := url.Values{}
		data.Set("data", "invalid_data")
		data.Set("signature", "invalid_sig")

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/liqpay", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("OtherError", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("ProcessWebhook", mock.Anything, "err_data", "err_sig").Return(assert.AnError).Once()

		data := url.Values{}
		data.Set("data", "err_data")
		data.Set("signature", "err_sig")

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/liqpay", strings.NewReader(data.Encode()))
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		// LiqPay вимагає 200 OK навіть при інших помилках
		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"error","message":"processing failed"}`, w.Body.String())
		svc.AssertExpectations(t)
	})
}

func TestPaymentHandler_HandleMockPayment(t *testing.T) {
	svc := new(mockPaymentService)
	r := setupTestRouter(svc)
	orderID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		os.Setenv("APP_ENV", "development")
		defer os.Unsetenv("APP_ENV")

		svc.ExpectedCalls = nil
		svc.On("SimulatePayment", mock.Anything, orderID).Return(nil).Once()

		body := map[string]string{"order_id": orderID.String()}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/mock-payment", bytes.NewBuffer(jsonBody))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("WrongEnv", func(t *testing.T) {
		os.Setenv("APP_ENV", "production")
		defer os.Unsetenv("APP_ENV")

		body := map[string]string{"order_id": orderID.String()}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/mock-payment", bytes.NewBuffer(jsonBody))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		os.Setenv("APP_ENV", "development")
		defer os.Unsetenv("APP_ENV")

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/mock-payment", strings.NewReader("invalid_json"))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		os.Setenv("APP_ENV", "development")
		defer os.Unsetenv("APP_ENV")

		body := map[string]string{"order_id": "not-a-uuid"}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/mock-payment", bytes.NewBuffer(jsonBody))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code) // Gin ShouldBindJSON will fail on binding required,uuid
	})
	
	t.Run("ServiceError", func(t *testing.T) {
		os.Setenv("APP_ENV", "development")
		defer os.Unsetenv("APP_ENV")

		svc.ExpectedCalls = nil
		svc.On("SimulatePayment", mock.Anything, orderID).Return(assert.AnError).Once()

		body := map[string]string{"order_id": orderID.String()}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/mock-payment", bytes.NewBuffer(jsonBody))
		req.Header.Add("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		svc.AssertExpectations(t)
	})
}

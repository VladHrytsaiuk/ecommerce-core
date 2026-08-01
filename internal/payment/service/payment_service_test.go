package service

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"go.uber.org/zap"
)

// --- Mocks ---

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

type mockPaymentRepo struct {
	mock.Mock
}

func (m *mockPaymentRepo) Create(ctx context.Context, payment *domain.Payment) error {
	args := m.Called(ctx, payment)
	return args.Error(0)
}

func (m *mockPaymentRepo) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	args := m.Called(ctx, orderID)
	var p *domain.Payment
	if args.Get(0) != nil {
		p = args.Get(0).(*domain.Payment)
	}
	return p, args.Error(1)
}

func (m *mockPaymentRepo) UpdateStatus(ctx context.Context, paymentID uuid.UUID, status, transactionID, errorMessage string) error {
	args := m.Called(ctx, paymentID, status, transactionID, errorMessage)
	return args.Error(0)
}

type mockOrderRepo struct {
	mock.Mock
}

func (m *mockOrderRepo) Create(ctx context.Context, order *orderDomain.Order, items []orderDomain.OrderItem, delivery *orderDomain.Delivery) error {
	args := m.Called(ctx, order, items, delivery)
	return args.Error(0)
}

func (m *mockOrderRepo) FindByID(ctx context.Context, id uuid.UUID) (*orderDomain.Order, error) {
	args := m.Called(ctx, id)
	var o *orderDomain.Order
	if args.Get(0) != nil {
		o = args.Get(0).(*orderDomain.Order)
	}
	return o, args.Error(1)
}

func (m *mockOrderRepo) FindByOrderNumber(ctx context.Context, orderNumber int64) (*orderDomain.Order, error) {
	args := m.Called(ctx, orderNumber)
	var o *orderDomain.Order
	if args.Get(0) != nil {
		o = args.Get(0).(*orderDomain.Order)
	}
	return o, args.Error(1)
}

func (m *mockOrderRepo) FindByUserID(ctx context.Context, userID uuid.UUID, p pagination.Params) ([]orderDomain.Order, int64, error) {
	return nil, 0, nil
}

func (m *mockOrderRepo) GetForUpdate(ctx context.Context, orderNumber int64) (*orderDomain.Order, error) {
	args := m.Called(ctx, orderNumber)
	var o *orderDomain.Order
	if args.Get(0) != nil {
		o = args.Get(0).(*orderDomain.Order)
	}
	return o, args.Error(1)
}

func (m *mockOrderRepo) GetOrderStatusByID(ctx context.Context, orderID uuid.UUID) (int, error) {
	args := m.Called(ctx, orderID)
	return args.Int(0), args.Error(1)
}

func (m *mockOrderRepo) GetOrdersForPaymentReminder(ctx context.Context, olderThan time.Time) ([]orderDomain.Order, error) {
	args := m.Called(ctx, olderThan)
	var orders []orderDomain.Order
	if args.Get(0) != nil {
		orders = args.Get(0).([]orderDomain.Order)
	}
	return orders, args.Error(1)
}

func (m *mockOrderRepo) GetOrdersForPaymentTimeout(ctx context.Context, olderThan time.Time) ([]orderDomain.Order, error) {
	args := m.Called(ctx, olderThan)
	var orders []orderDomain.Order
	if args.Get(0) != nil {
		orders = args.Get(0).([]orderDomain.Order)
	}
	return orders, args.Error(1)
}

func (m *mockOrderRepo) MarkPaymentReminderSent(ctx context.Context, orderID uuid.UUID) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *mockOrderRepo) SetManagerToken(ctx context.Context, orderID uuid.UUID, hash string, expiresAt time.Time) error {
	args := m.Called(ctx, orderID, hash, expiresAt)
	return args.Error(0)
}

func (m *mockOrderRepo) SetTTNData(ctx context.Context, orderID uuid.UUID, ttnNumber, ttnRef, carrierStatus string, rawResponse *string) error {
	args := m.Called(ctx, orderID, ttnNumber, ttnRef, carrierStatus, rawResponse)
	return args.Error(0)
}

func (m *mockOrderRepo) Update(ctx context.Context, order *orderDomain.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *mockOrderRepo) WithTransaction(ctx context.Context, fn func(ctx context.Context, txRepo orderDomain.OrderRepository) error) error {
	args := m.Called(ctx, fn)
	// For testing, just run the function without a real transaction
	if fn != nil {
		return fn(ctx, m)
	}
	return args.Error(0)
}

func (m *mockOrderRepo) CreateStatusHistory(ctx context.Context, history *orderDomain.OrderStatusHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

func (m *mockOrderRepo) FindAllOrders(ctx context.Context, filters orderDomain.AdminOrderFilters) ([]orderDomain.Order, int64, error) {
	args := m.Called(ctx, filters)
	var orders []orderDomain.Order
	if args.Get(0) != nil {
		orders = args.Get(0).([]orderDomain.Order)
	}
	return orders, args.Get(1).(int64), args.Error(2)
}

func (m *mockOrderRepo) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*orderDomain.Order, error) {
	args := m.Called(ctx, id)
	var o *orderDomain.Order
	if args.Get(0) != nil {
		o = args.Get(0).(*orderDomain.Order)
	}
	return o, args.Error(1)
}

func (m *mockOrderRepo) FindStatusHistory(ctx context.Context, orderID uuid.UUID) ([]orderDomain.OrderStatusHistory, error) {
	args := m.Called(ctx, orderID)
	var history []orderDomain.OrderStatusHistory
	if args.Get(0) != nil {
		history = args.Get(0).([]orderDomain.OrderStatusHistory)
	}
	return history, args.Error(1)
}

func (m *mockOrderRepo) GetOrderStatuses(ctx context.Context) ([]orderDomain.OrderStatus, error) {
	args := m.Called(ctx)
	var statuses []orderDomain.OrderStatus
	if args.Get(0) != nil {
		statuses = args.Get(0).([]orderDomain.OrderStatus)
	}
	return statuses, args.Error(1)
}

func (m *mockOrderRepo) GetShippedOrders(ctx context.Context) ([]orderDomain.Order, error) {
	args := m.Called(ctx)
	var orders []orderDomain.Order
	if args.Get(0) != nil {
		orders = args.Get(0).([]orderDomain.Order)
	}
	return orders, args.Error(1)
}

func (m *mockOrderRepo) UpdateAdminComment(ctx context.Context, orderID uuid.UUID, comment string) error {
	args := m.Called(ctx, orderID, comment)
	return args.Error(0)
}

func (m *mockOrderRepo) UpdateCarrierData(ctx context.Context, orderID uuid.UUID, carrierStatus string, rawResponse *string) error {
	args := m.Called(ctx, orderID, carrierStatus, rawResponse)
	return args.Error(0)
}


func (m *mockOrderRepo) UpdateStatus(ctx context.Context, id uuid.UUID, statusID int) error {
	args := m.Called(ctx, id, statusID)
	return args.Error(0)
}

type mockEmailProvider struct {
	mock.Mock
}

func (m *mockEmailProvider) SendEmail(ctx context.Context, to, subject, htmlBody string) error {
	args := m.Called(ctx, to, subject, htmlBody)
	return args.Error(0)
}

func (m *mockEmailProvider) SendAdminOrderNotification(to string, data email.OrderEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *mockEmailProvider) SendOrderConfirmationEmail(to string, data email.OrderEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *mockEmailProvider) SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
	args := m.Called(to, feedbackType, userEmail, content, mediaURL)
	return args.Error(0)
}

func (m *mockEmailProvider) SendPaymentReminderEmail(to string, data email.OrderEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *mockEmailProvider) SendPasswordResetEmail(to, code, link string) error {
	args := m.Called(to, code, link)
	return args.Error(0)
}

func (m *mockEmailProvider) SendSecurityWarningEmail(to string) error {
	args := m.Called(to)
	return args.Error(0)
}

func (m *mockEmailProvider) SendShipmentCreatedEmail(to string, data email.ShipmentEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *mockEmailProvider) SendVerificationEmail(to, code, link string) error {
	args := m.Called(to, code, link)
	return args.Error(0)
}

// --- Tests ---

func TestPaymentService_CreatePayment(t *testing.T) {
	repo := new(mockPaymentRepo)
	svc := NewPaymentService(repo, nil, nil, &config.Config{}, &mockLogger{})

	orderID := uuid.New()
	amount := 1000

	repo.On("Create", mock.Anything, mock.MatchedBy(func(p *domain.Payment) bool {
		return p.OrderID == orderID && p.Amount == amount && p.Status == "pending" && p.Provider == "liqpay"
	})).Return(nil)

	err := svc.CreatePayment(context.Background(), orderID, amount)
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestPaymentService_GeneratePaymentURL(t *testing.T) {
	cfg := &config.Config{
		LiqPayPublicKey:  "pub_key",
		LiqPayPrivateKey: "priv_key",
	}
	svc := NewPaymentService(nil, nil, nil, cfg, &mockLogger{})

	orderID := uuid.New()
	url := svc.GeneratePaymentURL(orderID, 15050, 100, "")
	assert.Contains(t, url, "https://www.liqpay.ua/api/3/checkout")
}

func TestPaymentService_SimulatePayment(t *testing.T) {
	paymentRepo := new(mockPaymentRepo)
	orderRepo := new(mockOrderRepo)
	emailProvider := new(mockEmailProvider)

	cfg := &config.Config{
		LiqPayPrivateKey: "priv",
	}
	svc := NewPaymentService(paymentRepo, orderRepo, emailProvider, cfg, &mockLogger{})

	orderID := uuid.New()
	paymentID := uuid.New()
	
	p := &domain.Payment{ID: paymentID, OrderID: orderID, Status: "pending"}
	o := &orderDomain.Order{ID: orderID, StatusID: orderDomain.StatusPendingPayment}

	paymentRepo.On("FindByOrderID", mock.Anything, orderID).Return(p, nil)
	paymentRepo.On("UpdateStatus", mock.Anything, paymentID, "success", mock.Anything, "").Return(nil)
	orderRepo.On("WithTransaction", mock.Anything, mock.Anything).Return(nil)
	// Inside transaction the mocks should be called, but wait, my mock implementation of WithTransaction just executes the function directly with `m` (which is `orderRepo`). So we need to mock what's inside.
	orderRepo.On("FindByID", mock.Anything, orderID).Return(o, nil)
	orderRepo.On("UpdateStatus", mock.Anything, orderID, orderDomain.StatusProcessing).Return(nil)
	orderRepo.On("CreateStatusHistory", mock.Anything, mock.Anything).Return(nil)
	orderRepo.On("SetManagerToken", mock.Anything, orderID, mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendAdminOrderNotification", mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendOrderConfirmationEmail", mock.Anything, mock.Anything).Return(nil)

	err := svc.SimulatePayment(context.Background(), orderID)
	require.NoError(t, err)
	
	time.Sleep(200 * time.Millisecond)

	paymentRepo.AssertExpectations(t)
	orderRepo.AssertExpectations(t)
}



func TestPaymentService_ProcessWebhook_Success(t *testing.T) {
	paymentRepo := new(mockPaymentRepo)
	orderRepo := new(mockOrderRepo)
	emailProvider := new(mockEmailProvider)

	cfg := &config.Config{
		LiqPayPrivateKey: "priv",
	}
	svc := NewPaymentService(paymentRepo, orderRepo, emailProvider, cfg, &mockLogger{})

	orderIDStr := uuid.New().String()
	orderID, _ := uuid.Parse(orderIDStr)
	paymentID := uuid.New()

	liqData := domain.LiqPayData{
		Action:   "pay",
		Status:   "success",
		OrderID:  orderIDStr,
		LiqPayID: 12345,
	}

	b, _ := json.Marshal(liqData)
	data64 := base64.StdEncoding.EncodeToString(b)
	
	h := sha1.New()
	h.Write([]byte(cfg.LiqPayPrivateKey + data64 + cfg.LiqPayPrivateKey))
	sig64 := base64.StdEncoding.EncodeToString(h.Sum(nil))

	p := &domain.Payment{ID: paymentID, OrderID: orderID, Status: "pending"}
	o := &orderDomain.Order{ID: orderID, StatusID: orderDomain.StatusPendingPayment, Email: "test@example.com"}

	paymentRepo.On("FindByOrderID", mock.Anything, orderID).Return(p, nil)
	paymentRepo.On("UpdateStatus", mock.Anything, paymentID, "success", "12345", "").Return(nil)
	orderRepo.On("WithTransaction", mock.Anything, mock.Anything).Return(nil)
	orderRepo.On("FindByID", mock.Anything, orderID).Return(o, nil)
	orderRepo.On("UpdateStatus", mock.Anything, orderID, orderDomain.StatusProcessing).Return(nil)
	orderRepo.On("CreateStatusHistory", mock.Anything, mock.Anything).Return(nil)
	orderRepo.On("SetManagerToken", mock.Anything, orderID, mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendAdminOrderNotification", mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendOrderConfirmationEmail", mock.Anything, mock.Anything).Return(nil)

	err := svc.ProcessWebhook(context.Background(), data64, sig64)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
}

func TestPaymentService_ProcessWebhook_InvalidSig(t *testing.T) {
	cfg := &config.Config{
		LiqPayPrivateKey: "priv",
	}
	svc := NewPaymentService(nil, nil, nil, cfg, &mockLogger{})

	err := svc.ProcessWebhook(context.Background(), "data", "invalid")
	require.ErrorIs(t, err, domain.ErrInvalidSignature)
}

func TestPaymentService_ProcessRefundStub(t *testing.T) {
	paymentRepo := new(mockPaymentRepo)
	svc := NewPaymentService(paymentRepo, nil, nil, &config.Config{}, &mockLogger{})

	orderID := uuid.New()
	paymentID := uuid.New()

	p := &domain.Payment{ID: paymentID, OrderID: orderID, Status: "success", Amount: 1000}
	paymentRepo.On("FindByOrderID", mock.Anything, orderID).Return(p, nil)
	paymentRepo.On("UpdateStatus", mock.Anything, paymentID, "refunded", mock.Anything, mock.Anything).Return(nil)

	err := svc.ProcessRefundStub(context.Background(), orderID)
	require.NoError(t, err)
}

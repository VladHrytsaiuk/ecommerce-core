package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	
	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	paymentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

// noopLogger для тестів
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

type MockOrderRepo struct {
	mock.Mock
	domain.OrderRepository
}

func (m *MockOrderRepo) Create(ctx context.Context, order *domain.Order, items []domain.OrderItem, delivery *domain.Delivery) error {
	args := m.Called(ctx, order, items, delivery)
	return args.Error(0)
}
func (m *MockOrderRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockOrderRepo) FindByOrderNumber(ctx context.Context, orderNumber int64) (*domain.Order, error) {
	args := m.Called(ctx, orderNumber)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockOrderRepo) GetOrderStatusByID(ctx context.Context, orderID uuid.UUID) (int, error) {
	args := m.Called(ctx, orderID)
	return args.Int(0), args.Error(1)
}
func (m *MockOrderRepo) GetForUpdate(ctx context.Context, orderNumber int64) (*domain.Order, error) {
	args := m.Called(ctx, orderNumber)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockOrderRepo) WithTransaction(ctx context.Context, fn func(ctx context.Context, txRepo domain.OrderRepository) error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}
func (m *MockOrderRepo) FindByUserID(ctx context.Context, userID uuid.UUID, pgn pagination.Params) ([]domain.Order, int64, error) {
	args := m.Called(ctx, userID, pgn)
	return args.Get(0).([]domain.Order), int64(args.Int(1)), args.Error(2)
}
func (m *MockOrderRepo) UpdateStatus(ctx context.Context, orderID uuid.UUID, statusID int) error {
	args := m.Called(ctx, orderID, statusID)
	return args.Error(0)
}
func (m *MockOrderRepo) Update(ctx context.Context, order *domain.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}
func (m *MockOrderRepo) SetManagerToken(ctx context.Context, orderID uuid.UUID, hash string, expiresAt time.Time) error {
	args := m.Called(ctx, orderID, hash, expiresAt)
	return args.Error(0)
}
func (m *MockOrderRepo) SetTTNData(ctx context.Context, orderID uuid.UUID, ttnNumber, ttnRef, carrierStatus string, rawResponse *string) error {
	args := m.Called(ctx, orderID, ttnNumber, ttnRef, carrierStatus, rawResponse)
	return args.Error(0)
}
func (m *MockOrderRepo) GetOrdersForPaymentReminder(ctx context.Context, olderThan time.Time) ([]domain.Order, error) {
	args := m.Called(ctx, olderThan)
	if args.Get(0) != nil {
		return args.Get(0).([]domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockOrderRepo) GetOrdersForPaymentTimeout(ctx context.Context, olderThan time.Time) ([]domain.Order, error) {
	args := m.Called(ctx, olderThan)
	if args.Get(0) != nil {
		return args.Get(0).([]domain.Order), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *MockOrderRepo) MarkPaymentReminderSent(ctx context.Context, orderID uuid.UUID) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}
func (m *MockOrderRepo) CreateStatusHistory(ctx context.Context, history *domain.OrderStatusHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

type MockPaymentSvc struct {
	mock.Mock
	paymentDomain.PaymentService
}

func (m *MockPaymentSvc) GeneratePaymentURL(orderID uuid.UUID, amount int, orderNumber int64, payTypes string) string {
	args := m.Called(orderID, amount, orderNumber, payTypes)
	return args.String(0)
}
func (m *MockPaymentSvc) ProcessRefundStub(ctx context.Context, orderID uuid.UUID) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

type MockEmailProvider struct {
	mock.Mock
	email.Provider
}

func (m *MockEmailProvider) SendPaymentReminderEmail(to string, data email.OrderEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *MockEmailProvider) SendShipmentCreatedEmail(to string, data email.ShipmentEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func setupOrderService() (*MockOrderRepo, *MockPaymentSvc, *MockEmailProvider, domain.OrderService) {
	mockRepo := new(MockOrderRepo)
	mockPayment := new(MockPaymentSvc)
	mockEmail := new(MockEmailProvider)

	cfg := &config.Config{FrontendURL: "http://localhost:3000"}
	loggerInstance := &noopLogger{}

	svc := NewOrderService(
		mockRepo,
		nil, // cartRepo
		nil, // userRepo
		nil, // verifyCodeRepo
		nil, // promoRepo
		nil, // promoService
		mockPayment, // paymentService
		nil, // shipmentService
		mockEmail,
		cfg,
		loggerInstance,
	)

	return mockRepo, mockPayment, mockEmail, svc
}

func TestGetByID(t *testing.T) {
	mockRepo, _, _, svc := setupOrderService()
	ctx := context.Background()
	orderID := uuid.New()

	expectedOrder := &domain.Order{ID: orderID}
	mockRepo.On("FindByID", ctx, orderID).Return(expectedOrder, nil)

	order, err := svc.GetByID(ctx, orderID)
	require.NoError(t, err)
	assert.Equal(t, expectedOrder, order)
}

func TestCancelOrderByUser(t *testing.T) {
	mockRepo, mockPayment, _, svc := setupOrderService()
	ctx := context.Background()
	orderID := uuid.New()
	userID := uuid.New()

	order := &domain.Order{
		ID:       orderID,
		UserID:   &userID,
		StatusID: domain.StatusPaid, // should trigger refund
	}

	mockRepo.On("FindByID", ctx, orderID).Return(order, nil)
	mockRepo.On("UpdateStatus", ctx, orderID, domain.StatusCancelled).Return(nil)
	mockRepo.On("CreateStatusHistory", ctx, mock.Anything).Return(nil)
	mockPayment.On("ProcessRefundStub", ctx, orderID).Return(nil)

	err := svc.CancelOrderByUser(ctx, userID, orderID)
	require.NoError(t, err)

	mockRepo.AssertExpectations(t)
	mockPayment.AssertExpectations(t)
}

func TestProcessPaymentTimeouts(t *testing.T) {
	mockRepo, mockPayment, mockEmail, svc := setupOrderService()
	ctx := context.Background()

	orderToRemind := domain.Order{
		ID:          uuid.New(),
		OrderNumber: 123,
		Email:       "test@example.com",
		TotalPrice:  1000,
	}

	orderToCancel := domain.Order{
		ID: uuid.New(),
	}

	mockRepo.On("GetOrdersForPaymentReminder", ctx, mock.Anything).Return([]domain.Order{orderToRemind}, nil)
	mockRepo.On("GetOrdersForPaymentTimeout", ctx, mock.Anything).Return([]domain.Order{orderToCancel}, nil)

	mockPayment.On("GeneratePaymentURL", orderToRemind.ID, mock.Anything, int64(123), mock.Anything).Return("http://pay.url")
	mockEmail.On("SendPaymentReminderEmail", "test@example.com", mock.Anything).Return(nil)
	mockRepo.On("MarkPaymentReminderSent", ctx, orderToRemind.ID).Return(nil)

	mockRepo.On("UpdateStatus", ctx, orderToCancel.ID, domain.StatusCancelled).Return(nil)
	mockRepo.On("CreateStatusHistory", ctx, mock.Anything).Return(nil)

	err := svc.ProcessPaymentTimeouts(ctx)
	require.NoError(t, err)

	mockRepo.AssertExpectations(t)
	mockPayment.AssertExpectations(t)
	mockEmail.AssertExpectations(t)
}

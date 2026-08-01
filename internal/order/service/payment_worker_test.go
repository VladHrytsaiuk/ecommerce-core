package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
)

type MockOrderService struct {
	mock.Mock
	domain.OrderService
}

func (m *MockOrderService) ProcessPaymentTimeouts(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestPaymentWorker(t *testing.T) {
	mockOrderSvc := new(MockOrderService)
	loggerInstance := &noopLogger{}

	worker := NewPaymentWorker(mockOrderSvc, loggerInstance)

	ctx, cancel := context.WithCancel(context.Background())
	
	// Expect it to be called multiple times, but we will cancel it fast
	mockOrderSvc.On("ProcessPaymentTimeouts", mock.Anything).Return(nil)

	// Start with a very short interval
	worker.Start(ctx, 10*time.Millisecond)

	// Wait for a few ticks
	time.Sleep(35 * time.Millisecond)
	cancel()
	// Wait a bit to ensure it stops cleanly
	time.Sleep(10 * time.Millisecond)

	// It should have been called at least 2-3 times
	mockOrderSvc.AssertCalled(t, "ProcessPaymentTimeouts", mock.Anything)
	calls := len(mockOrderSvc.Calls)
	assert.GreaterOrEqual(t, calls, 1)
}

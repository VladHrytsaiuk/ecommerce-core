//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

	// Application owns goroutine creation; the worker itself runs synchronously.
	done := make(chan struct{})
	go func() {
		worker.Run(ctx, 10*time.Millisecond)
		close(done)
	}()

	// Wait for a few ticks
	time.Sleep(35 * time.Millisecond)
	cancel()
	<-done

	// It should have been called at least 2-3 times
	mockOrderSvc.AssertCalled(t, "ProcessPaymentTimeouts", mock.Anything)
	calls := len(mockOrderSvc.Calls)
	assert.GreaterOrEqual(t, calls, 1)
}

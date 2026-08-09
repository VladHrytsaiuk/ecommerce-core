package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// RecoveryService recovers pending checkout attempts that were interrupted
// by network timeouts or application crashes.
type RecoveryService struct {
	workflow recoveryWorkflowPort
	gateways interface {
		Get(string) (paymentsDomain.Gateway, bool)
	}
	logger logger.Logger
}

type recoveryWorkflowPort interface {
	ClaimPendingCheckoutAttempt(context.Context, time.Duration, time.Duration) (*domain.CheckoutAttempt, error)
	RetryCheckoutAttempt(context.Context, uuid.UUID, error) error
	MarkCheckoutAttemptFailed(context.Context, uuid.UUID) error
	CancelPending(context.Context, uuid.UUID) error
	RegisterPayment(context.Context, domain.PaymentAttempt) error
}

func NewRecoveryService(workflow recoveryWorkflowPort, gateways interface {
	Get(string) (paymentsDomain.Gateway, bool)
}, logger logger.Logger) *RecoveryService {
	return &RecoveryService{
		workflow: workflow,
		gateways: gateways,
		logger:   logger,
	}
}

// RecoverPendingCheckouts polls the database for creating attempts older than the given duration
// and calls the gateway with the same idempotency key to complete the payment registration.
func (s *RecoveryService) RecoverOnce(ctx context.Context, olderThan, lease time.Duration) error {
	attempt, err := s.workflow.ClaimPendingCheckoutAttempt(ctx, olderThan, lease)
	if err != nil || attempt == nil {
		return err
	}
	gateway, enabled := s.gateways.Get(attempt.Provider)
	if !enabled {
		return s.workflow.RetryCheckoutAttempt(context.WithoutCancel(ctx), attempt.OrderID, fmt.Errorf("payment gateway %q is not enabled", attempt.Provider))
	}

	s.logger.Infow("recovering checkout attempt", "order_id", attempt.OrderID, "idempotency_key", attempt.IdempotencyKey)

	// Re-invoke gateway checkout creation using the stable idempotency key
	session, err := gateway.CreateCheckout(ctx, paymentsDomain.CheckoutPayment{
		OrderID:        attempt.OrderID,
		IdempotencyKey: attempt.IdempotencyKey,
		Amount:         attempt.Amount,
		// Recovery doesn't have original return/cancel URLs, but the gateway
		// will likely return the existing intent anyway which already has them.
		ReturnURL: "",
		CancelURL: "",
	})
	if err != nil {
		if errors.Is(err, paymentsDomain.ErrGatewayRejected) {
			if markErr := s.workflow.MarkCheckoutAttemptFailed(context.WithoutCancel(ctx), attempt.OrderID); markErr != nil {
				return fmt.Errorf("mark rejected checkout attempt failed: %w", markErr)
			}
			return s.workflow.CancelPending(context.WithoutCancel(ctx), attempt.OrderID)
		}
		s.logger.Errorw("failed to recover checkout attempt", "order_id", attempt.OrderID, "error", err)
		return s.workflow.RetryCheckoutAttempt(context.WithoutCancel(ctx), attempt.OrderID, err)
	}

	// Gateway succeeded (returned existing intent or created a new one safely)
	// Register it to complete the cycle and move attempt to 'created'
	err = s.workflow.RegisterPayment(ctx, domain.PaymentAttempt{
		OrderID:           attempt.OrderID,
		Provider:          gateway.Code(),
		ProviderReference: session.ProviderReference,
		Amount:            attempt.Amount,
	})
	if err != nil {
		s.logger.Errorw("failed to register recovered payment", "order_id", attempt.OrderID, "error", err)
		return s.workflow.RetryCheckoutAttempt(context.WithoutCancel(ctx), attempt.OrderID, err)
	}
	s.logger.Infow("successfully recovered and registered checkout", "order_id", attempt.OrderID)
	return nil
}

func (s *RecoveryService) Run(ctx context.Context, interval, olderThan, lease time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	for {
		_ = s.RecoverOnce(ctx, olderThan, lease)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

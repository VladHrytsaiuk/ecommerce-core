package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	workflowDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/orderworkflow/domain"
	ordersApp "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/application"
	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
)

type Service struct {
	repo workflowDomain.Repository
}

func NewService(repo workflowDomain.Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreatePending(ctx context.Context, draft ordersDomain.Draft, reservationIDs []uuid.UUID) (*ordersDomain.Order, error) {
	if err := validateReservationIDs(reservationIDs); err != nil {
		return nil, err
	}
	order, err := ordersApp.NewPendingOrder(draft)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreatePending(ctx, order, reservationIDs); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) CancelPending(ctx context.Context, orderID uuid.UUID) error {
	if orderID == uuid.Nil {
		return fmt.Errorf("invalid order id")
	}
	return s.repo.CancelPending(ctx, orderID)
}

func (s *Service) RegisterPayment(ctx context.Context, attempt workflowDomain.PaymentAttempt) error {
	if err := validatePaymentAttempt(attempt); err != nil {
		return err
	}
	return s.repo.RegisterPayment(ctx, attempt)
}

func (s *Service) MarkPaid(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	if err := validatePaymentConfirmation(confirmation, "paid"); err != nil {
		return err
	}
	return s.repo.MarkPaid(ctx, confirmation)
}

func (s *Service) MarkFailed(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	if err := validatePaymentConfirmation(confirmation, "failed"); err != nil {
		return err
	}
	return s.repo.MarkFailed(ctx, confirmation)
}

func validateReservationIDs(reservationIDs []uuid.UUID) error {
	if len(reservationIDs) == 0 {
		return fmt.Errorf("at least one reservation is required")
	}
	seen := make(map[uuid.UUID]struct{}, len(reservationIDs))
	for _, reservationID := range reservationIDs {
		if reservationID == uuid.Nil {
			return fmt.Errorf("invalid reservation id")
		}
		if _, exists := seen[reservationID]; exists {
			return fmt.Errorf("duplicate reservation id")
		}
		seen[reservationID] = struct{}{}
	}
	return nil
}

func validatePaymentAttempt(attempt workflowDomain.PaymentAttempt) error {
	if attempt.OrderID == uuid.Nil || strings.TrimSpace(attempt.Provider) == "" || strings.TrimSpace(attempt.ProviderReference) == "" || attempt.Amount.Amount <= 0 || attempt.Amount.Currency == "" {
		return fmt.Errorf("invalid payment attempt")
	}
	return nil
}

func validatePaymentConfirmation(confirmation workflowDomain.PaymentConfirmation, expectedStatus string) error {
	if confirmation.Status != expectedStatus {
		return fmt.Errorf("invalid payment confirmation status")
	}
	return validatePaymentAttempt(confirmation.PaymentAttempt)
}

var _ workflowDomain.Service = (*Service)(nil)

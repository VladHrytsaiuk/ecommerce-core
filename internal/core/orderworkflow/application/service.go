package application

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func (s *Service) CreatePendingCheckout(ctx context.Context, draft ordersDomain.Draft, reservationIDs []uuid.UUID, attempt workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	if err := validateReservationIDs(reservationIDs); err != nil {
		return nil, err
	}
	if draft.ExpiresAt.IsZero() {
		draft.ExpiresAt = time.Now().UTC().Add(30 * time.Minute)
	}
	if attempt.ExpiresAt.IsZero() {
		attempt.ExpiresAt = draft.ExpiresAt
	}
	if attempt.OrderID != uuid.Nil || strings.TrimSpace(attempt.Provider) == "" || strings.TrimSpace(attempt.IdempotencyKey) == "" || attempt.Amount.Amount() <= 0 || attempt.Amount.Validate() != nil || attempt.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("invalid checkout attempt request")
	}
	if draft.Contact == nil || strings.TrimSpace(draft.Contact.Email) == "" || strings.TrimSpace(draft.Contact.Locale) == "" {
		return nil, fmt.Errorf("checkout contact is required")
	}
	order, err := ordersApp.NewPendingOrder(draft)
	if err != nil {
		return nil, err
	}
	attempt.OrderID = order.ID
	if err := s.repo.CreatePendingCheckout(ctx, order, reservationIDs, attempt); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) CreatePaidCheckout(ctx context.Context, draft ordersDomain.Draft, reservationIDs []uuid.UUID, attempt workflowDomain.CheckoutAttemptRequest) (*ordersDomain.Order, error) {
	if err := validateReservationIDs(reservationIDs); err != nil {
		return nil, err
	}
	if draft.ExpiresAt.IsZero() {
		draft.ExpiresAt = time.Now().UTC().Add(30 * time.Minute)
	}
	if attempt.ExpiresAt.IsZero() {
		attempt.ExpiresAt = draft.ExpiresAt
	}
	if attempt.OrderID != uuid.Nil || attempt.Provider != "free" || strings.TrimSpace(attempt.IdempotencyKey) == "" || attempt.Amount.Amount() != 0 || attempt.Amount.Validate() != nil || attempt.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("invalid free checkout attempt request")
	}
	if draft.Contact == nil || strings.TrimSpace(draft.Contact.Email) == "" || strings.TrimSpace(draft.Contact.Locale) == "" {
		return nil, fmt.Errorf("checkout contact is required")
	}
	order, err := ordersApp.NewPendingOrder(draft)
	if err != nil {
		return nil, err
	}
	attempt.OrderID = order.ID
	if err := s.repo.CreatePaidCheckout(ctx, order, reservationIDs, attempt); err != nil {
		return nil, err
	}
	order.Status = ordersDomain.StatusPaid
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
func (s *Service) MarkRefunded(ctx context.Context, confirmation workflowDomain.PaymentConfirmation) error {
	if err := validatePaymentConfirmation(confirmation, "refunded"); err != nil {
		return err
	}
	return s.repo.MarkRefunded(ctx, confirmation)
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
	if attempt.OrderID == uuid.Nil || strings.TrimSpace(attempt.Provider) == "" || strings.TrimSpace(attempt.ProviderReference) == "" || attempt.Amount.Amount() <= 0 || attempt.Amount.Validate() != nil {
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

func (s *Service) RecordCheckoutAttempt(ctx context.Context, req workflowDomain.CheckoutAttemptRequest) error {
	if req.OrderID == uuid.Nil || strings.TrimSpace(req.Provider) == "" || strings.TrimSpace(req.IdempotencyKey) == "" || req.Amount.Amount() <= 0 || req.Amount.Validate() != nil {
		return fmt.Errorf("invalid checkout attempt request")
	}
	return s.repo.RecordCheckoutAttempt(ctx, req)
}

func (s *Service) FindCheckoutAttempt(ctx context.Context, idempotencyKey string) (*workflowDomain.CheckoutAttempt, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, fmt.Errorf("checkout idempotency key is required")
	}
	return s.repo.FindCheckoutAttempt(ctx, idempotencyKey)
}

func (s *Service) ClaimPendingCheckoutAttempt(ctx context.Context, olderThan, lease time.Duration) (*workflowDomain.CheckoutAttempt, error) {
	if olderThan <= 0 || lease <= 0 {
		return nil, fmt.Errorf("checkout attempt recovery age and lease must be positive")
	}
	return s.repo.ClaimPendingCheckoutAttempt(ctx, olderThan, lease)
}

func (s *Service) ExpirePendingCheckout(ctx context.Context, now time.Time) (bool, error) {
	if now.IsZero() {
		return false, fmt.Errorf("expiry time is required")
	}
	return s.repo.ExpirePendingCheckout(ctx, now)
}

func (s *Service) MarkCheckoutAttemptFailed(ctx context.Context, orderID uuid.UUID) error {
	if orderID == uuid.Nil {
		return fmt.Errorf("invalid order id")
	}
	return s.repo.MarkCheckoutAttemptFailed(ctx, orderID)
}

func (s *Service) RetryCheckoutAttempt(ctx context.Context, orderID uuid.UUID, cause error) error {
	if orderID == uuid.Nil || cause == nil {
		return fmt.Errorf("invalid checkout retry")
	}
	return s.repo.RetryCheckoutAttempt(ctx, orderID, cause)
}

var _ workflowDomain.Service = (*Service)(nil)

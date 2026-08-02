package application

import (
	"context"
	"fmt"

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

func (s *Service) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	if orderID == uuid.Nil {
		return fmt.Errorf("invalid order id")
	}
	return s.repo.MarkPaid(ctx, orderID)
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

var _ workflowDomain.Service = (*Service)(nil)

//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"errors"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	paymentDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"github.com/google/uuid"
)

type adminOrderService struct {
	repo           domain.OrderRepository
	confirmService domain.ConfirmOrderService
	paymentService paymentDomain.PaymentService
	paymentRepo    paymentDomain.PaymentRepository
	l              logger.Logger
}

// NewAdminOrderService створює сервіс для адміністрування замовлень
func NewAdminOrderService(repo domain.OrderRepository, confirmSvc domain.ConfirmOrderService, paymentSvc paymentDomain.PaymentService, paymentRepo paymentDomain.PaymentRepository, l logger.Logger) domain.AdminOrderService {
	return &adminOrderService{
		repo:           repo,
		confirmService: confirmSvc,
		paymentService: paymentSvc,
		paymentRepo:    paymentRepo,
		l:              l,
	}
}

func (s *adminOrderService) ListOrders(ctx context.Context, filters domain.AdminOrderFilters) ([]domain.Order, pagination.Metadata, error) {
	orders, total, err := s.repo.FindAllOrders(ctx, filters)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}
	meta := pagination.CalculateMetadata(total, filters.Page, filters.Limit)
	return orders, meta, nil
}

func (s *adminOrderService) GetOrderByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, []domain.OrderStatusHistory, *domain.AdminPaymentInfo, error) {
	order, err := s.repo.FindByID(ctx, orderID)
	if err != nil {
		return nil, nil, nil, err
	}

	history, err := s.repo.FindStatusHistory(ctx, orderID)
	if err != nil {
		s.l.Errorw("failed to get status history for admin", "error", err, "order_id", orderID)
		return nil, nil, nil, err
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil && !errors.Is(err, paymentDomain.ErrPaymentNotFound) {
		s.l.Errorw("failed to get payment for admin order", "error", err, "order_id", orderID)
		return nil, nil, nil, err
	}

	if payment == nil {
		return order, history, nil, nil
	}

	return order, history, &domain.AdminPaymentInfo{
		Provider:      payment.Provider,
		Status:        payment.Status,
		TransactionID: payment.TransactionID,
		Amount:        payment.Amount,
		Currency:      payment.Currency,
	}, nil
}

func (s *adminOrderService) GetOrderStatuses(ctx context.Context) ([]domain.OrderStatus, error) {
	return s.repo.GetOrderStatuses(ctx)
}

func (s *adminOrderService) UpdateAdminComment(ctx context.Context, orderID uuid.UUID, comment string) error {
	return s.repo.UpdateAdminComment(ctx, orderID, comment)
}

func (s *adminOrderService) ConfirmOrder(ctx context.Context, orderID uuid.UUID, adminUserID uuid.UUID) (*domain.Order, error) {
	// Використовуємо спільну бізнес-логіку підтвердження
	return s.confirmService.ConfirmOrder(ctx, orderID, domain.SourceAdmin, &adminUserID)
}

func (s *adminOrderService) CancelOrder(ctx context.Context, orderID uuid.UUID, adminUserID uuid.UUID) error {
	var oldOrder *domain.Order

	// Заблокуємо замовлення і перевіримо можливість скасування
	err := s.repo.WithTransaction(ctx, func(txCtx context.Context, txRepo domain.OrderRepository) error {
		order, err := txRepo.FindByIDForUpdate(txCtx, orderID)
		if err != nil {
			return err
		}

		oldOrder = order

		if order.StatusID == domain.StatusCancelled {
			return domain.ErrOrderAlreadyCancelled
		}

		if order.TTNNumber != "" {
			return domain.ErrCancelBlockedAfterTTN
		}

		if err := domain.ValidateTransition(order.StatusID, domain.StatusCancelled); err != nil {
			return err
		}

		if err := txRepo.UpdateStatus(txCtx, order.ID, domain.StatusCancelled); err != nil {
			return err
		}

		// Історія
		if err := txRepo.CreateStatusHistory(txCtx, &domain.OrderStatusHistory{
			OrderID:      order.ID,
			FromStatusID: &order.StatusID,
			ToStatusID:   domain.StatusCancelled,
			Source:       domain.SourceAdmin,
			AdminUserID:  &adminUserID,
			Comment:      ptr("Скасовано адміністратором"),
		}); err != nil {
			return err
		}
		s.l.Infow("Order cancelled by admin", "order_id", orderID, "admin_id", adminUserID)
		return nil
	})
	if err != nil {
		return err
	}

	// Якщо замовлення було оплачено, ініціюємо повернення
	if oldOrder != nil && (oldOrder.StatusID == domain.StatusPaid || oldOrder.StatusID == domain.StatusProcessing || oldOrder.StatusID == domain.StatusShipped) {
		if refundErr := s.paymentService.ProcessRefundStub(ctx, orderID); refundErr != nil {
			s.l.Errorw("Failed to process refund during admin cancellation", "error", refundErr, "order_id", orderID)
		}
	}

	return nil
}

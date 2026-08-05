//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"time"

	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

type managerService struct {
	orderRepo      orderDomain.OrderRepository
	confirmService orderDomain.ConfirmOrderService
	l              logger.Logger
}

// NewManagerService створює новий сервіс для менеджерських операцій
func NewManagerService(
	orderRepo orderDomain.OrderRepository,
	confirmService orderDomain.ConfirmOrderService,
	l logger.Logger,
) orderDomain.ManagerService {
	return &managerService{
		orderRepo:      orderRepo,
		confirmService: confirmService,
		l:              l,
	}
}

// GetOrderByToken повертає замовлення після валідації менеджерського токена
func (s *managerService) GetOrderByToken(ctx context.Context, orderNumber int64, plainToken string) (*orderDomain.Order, error) {
	order, err := s.validateToken(ctx, orderNumber, plainToken)
	if err != nil {
		return nil, err
	}
	return order, nil
}

// ConfirmOrder підтверджує замовлення та створює ТТН (делегує до ConfirmOrderService)
func (s *managerService) ConfirmOrder(ctx context.Context, orderNumber int64, plainToken string) (*orderDomain.Order, error) {
	// 1. Попередня валідація токена
	order, err := s.validateToken(ctx, orderNumber, plainToken)
	if err != nil {
		return nil, err
	}

	// 2. Делегуємо виконання спільній бізнес-логіці
	return s.confirmService.ConfirmOrder(ctx, order.ID, orderDomain.SourceManagerLink, nil)
}

func (s *managerService) CancelOrder(ctx context.Context, orderNumber int64, plainToken string) error {
	order, err := s.validateToken(ctx, orderNumber, plainToken)
	if err != nil {
		return err
	}

	if order.StatusID == orderDomain.StatusCancelled {
		return orderDomain.ErrOrderAlreadyCancelled
	}

	if order.TTNNumber != "" {
		return orderDomain.ErrCancelBlockedAfterTTN
	}

	if err := orderDomain.ValidateTransition(order.StatusID, orderDomain.StatusCancelled); err != nil {
		return err
	}

	if err := s.orderRepo.UpdateStatus(ctx, order.ID, orderDomain.StatusCancelled); err != nil {
		s.l.Errorw("Failed to cancel order", "error", err, "order_id", order.ID)
		return err
	}

	// Записуємо історію скасування
	if err := s.orderRepo.CreateStatusHistory(ctx, &orderDomain.OrderStatusHistory{
		OrderID:      order.ID,
		FromStatusID: &order.StatusID,
		ToStatusID:   orderDomain.StatusCancelled,
		Source:       orderDomain.SourceManagerLink,
	}); err != nil {
		s.l.Errorw("failed to create status history for cancellation", "error", err, "order_id", order.ID)
		return err
	}

	s.l.Infow("Order cancelled by manager", "order_number", orderNumber)

	return nil
}

// validateToken перевіряє менеджерський токен: знаходить замовлення, порівнює hash, перевіряє expiration
func (s *managerService) validateToken(ctx context.Context, orderNumber int64, plainToken string) (*orderDomain.Order, error) {
	if plainToken == "" {
		return nil, orderDomain.ErrInvalidManagerToken
	}

	order, err := s.orderRepo.FindByOrderNumber(ctx, orderNumber)
	if err != nil {
		return nil, err
	}

	// Перевірка наявності токена
	if order.ManagerTokenHash == "" {
		s.l.Warnw("Manager token not set for order", "order_number", orderNumber)
		return nil, orderDomain.ErrInvalidManagerToken
	}

	// Перевірка терміну дії
	if order.ManagerTokenExpiresAt == nil || time.Now().After(*order.ManagerTokenExpiresAt) {
		s.l.Warnw("Manager token expired", "order_number", orderNumber)
		return nil, orderDomain.ErrInvalidManagerToken
	}

	// Constant-time порівняння хешів
	providedHash := orderDomain.HashManagerToken(plainToken)
	if !orderDomain.ConstantTimeCompareHash(providedHash, order.ManagerTokenHash) {
		s.l.Warnw("Invalid manager token hash", "order_number", orderNumber)
		return nil, orderDomain.ErrInvalidManagerToken
	}

	return order, nil
}

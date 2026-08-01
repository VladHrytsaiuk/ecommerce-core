package service

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	shippingDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/shipping/domain"
	"go.uber.org/zap"
)

type confirmService struct {
	orderRepo      orderDomain.OrderRepository
	carrierService shippingDomain.CarrierService
	emailProvider  email.Provider
	cfg            *config.Config
	l              logger.Logger
}

// NewConfirmService створює спільний сервіс підтвердження замовлень.
// Використовується як Manager, так і Admin flow.
func NewConfirmService(
	orderRepo orderDomain.OrderRepository,
	carrierService shippingDomain.CarrierService,
	emailProvider email.Provider,
	cfg *config.Config,
	l logger.Logger,
) orderDomain.ConfirmOrderService {
	return &confirmService{
		orderRepo:      orderRepo,
		carrierService: carrierService,
		emailProvider:  emailProvider,
		cfg:            cfg,
		l:              l,
	}
}

// ConfirmOrder — єдина бізнес-операція підтвердження замовлення.
// Атомарно: блокує замовлення → створює ТТН → зберігає дані → Processing→Shipped → історія.
//
// Конкурентність MVP: SELECT FOR UPDATE тримає блокування рядка ~1-3с (час виклику НП).
// Конкурентні запити просто чекають. Для high-load можна пізніше винести на confirmation_in_progress flag.
func (s *confirmService) ConfirmOrder(ctx context.Context, orderID uuid.UUID, source string, adminUserID *uuid.UUID) (*orderDomain.Order, error) {
	var updatedOrder *orderDomain.Order

	err := s.orderRepo.WithTransaction(ctx, func(txCtx context.Context, txRepo orderDomain.OrderRepository) error {
		// 1. Блокуємо замовлення (SELECT ... FOR UPDATE)
		order, err := txRepo.FindByIDForUpdate(txCtx, orderID)
		if err != nil {
			return err
		}

		// 2. Ідемпотентність: якщо ТТН вже створена — повертаємо замовлення без помилки
		if order.TTNNumber != "" {
			s.l.Infow("ConfirmOrder: TTN already exists (idempotent return)",
				"order_id", orderID,
				"ttn_number", order.TTNNumber,
			)
			updatedOrder = order
			return nil
		}

		// 3. Валідація переходу: Processing → Shipped
		if err := orderDomain.ValidateTransition(order.StatusID, orderDomain.StatusShipped); err != nil {
			if order.StatusID != orderDomain.StatusProcessing {
				return orderDomain.ErrOrderNotProcessing
			}
			return err
		}

		// 4. Перевірка наявності delivery даних
		if order.Delivery == nil {
			return fmt.Errorf("order %s has no delivery data", orderID)
		}

		// 5. Розрахунок ваги
		var totalWeight float64
		for _, item := range order.Items {
			itemWeight := item.Variation.Weight
			if itemWeight <= 0 {
				itemWeight = 0.5 // Default fallback
			}
			totalWeight += itemWeight * float64(item.Quantity)
		}
		// Додаємо 0.5 кг на упаковку та округлюємо до більшого кратного 0.5
		totalWeight += 0.5
		roundedWeight := math.Ceil(totalWeight*2) / 2
		weightStr := fmt.Sprintf("%.1f", roundedWeight)

		// 6. Створюємо ТТН через CarrierService
		description := fmt.Sprintf("Замовлення №%d — AquaWheel Store", order.OrderNumber)
		result, err := s.carrierService.CreateShipment(ctx, shippingDomain.CreateShipmentRequest{
			Provider:       order.Delivery.Provider,
			RecipientName:  order.LastName + " " + order.FirstName,
			RecipientPhone: order.Phone,
			CityRef:        order.Delivery.CityRef,
			WarehouseRef:   order.Delivery.WarehouseRef,
			Weight:         weightStr,
			Description:    description,
			DeclaredValue:  order.TotalPrice,
		})
		if err != nil {
			s.l.Errorw("ConfirmOrder: failed to create shipment",
				"error", err,
				"order_id", orderID,
				"provider", order.Delivery.Provider,
			)
			return err
		}

		// 7. Зберігаємо дані ТТН (БЕЗ зміни статусу — це робить SetTTNData без status_id)
		rawResp := &result.RawResponse
		if err := txRepo.SetTTNData(txCtx, order.ID, result.TrackingNumber, result.Ref, "created", rawResp); err != nil {
			s.l.Errorw("ConfirmOrder: failed to save TTN data", "error", err, "order_id", order.ID)
			return err
		}

		// 8. Змінюємо статус на Shipped
		if err := txRepo.UpdateStatus(txCtx, order.ID, orderDomain.StatusShipped); err != nil {
			s.l.Errorw("ConfirmOrder: failed to update status to Shipped", "error", err, "order_id", order.ID)
			return err
		}

		// 9. Записуємо в історію: Processing → Shipped
		fromStatus := orderDomain.StatusProcessing
		if err := txRepo.CreateStatusHistory(txCtx, &orderDomain.OrderStatusHistory{
			OrderID:      order.ID,
			FromStatusID: &fromStatus,
			ToStatusID:   orderDomain.StatusShipped,
			Source:       source,
			AdminUserID:  adminUserID,
		}); err != nil {
			s.l.Errorw("ConfirmOrder: failed to create status history", "error", err, "order_id", order.ID)
			return err
		}

		s.l.Infow("ConfirmOrder: order confirmed, TTN created",
			"order_id", orderID,
			"ttn_number", result.TrackingNumber,
			"ttn_ref", result.Ref,
			"source", source,
		)

		// 10. Async: надсилаємо email клієнту з ТТН
		go func(o *orderDomain.Order, r *shippingDomain.ShipmentResult) {
			emailData := email.ShipmentEmailData{
				OrderNumber:    o.OrderNumber,
				CustomerName:   o.FirstName,
				Provider:       r.Provider,
				TrackingNumber: r.TrackingNumber,
				CityName:       o.Delivery.CityName,
				WarehouseName:  o.Delivery.WarehouseName,
			}
			if err := s.emailProvider.SendShipmentCreatedEmail(o.Email, emailData); err != nil {
				s.l.Error("ConfirmOrder: failed to send shipment email",
					zap.Error(err),
					zap.String("email", o.Email),
					zap.Int64("order_number", o.OrderNumber),
				)
			}
		}(order, result)

		// Перечитуємо замовлення для актуальних даних
		updatedOrder, err = txRepo.FindByOrderNumber(txCtx, order.OrderNumber)
		return err
	})

	if err != nil {
		return nil, err
	}

	return updatedOrder, nil
}

//go:build legacy && ignore
// +build legacy,ignore

package service

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	orderDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type paymentService struct {
	paymentRepo   domain.PaymentRepository
	orderRepo     orderDomain.OrderRepository
	emailProvider email.Provider
	cfg           *config.Config
	l             logger.Logger
}

// NewPaymentService створює новий сервіс платежів
func NewPaymentService(
	paymentRepo domain.PaymentRepository,
	orderRepo orderDomain.OrderRepository,
	emailProvider email.Provider,
	cfg *config.Config,
	l logger.Logger,
) domain.PaymentService {
	return &paymentService{
		paymentRepo:   paymentRepo,
		orderRepo:     orderRepo,
		emailProvider: emailProvider,
		cfg:           cfg,
		l:             l,
	}
}

// CreatePayment створює порожній запис Payment для замовлення
func (s *paymentService) CreatePayment(ctx context.Context, orderID uuid.UUID, amount int) error {
	payment := &domain.Payment{
		OrderID:  orderID,
		Provider: "liqpay",
		Amount:   amount,
		Currency: "UAH",
		Status:   "pending",
	}
	return s.paymentRepo.Create(ctx, payment)
}

// ProcessWebhook обробляє callback від LiqPay
func (s *paymentService) ProcessWebhook(ctx context.Context, data, signature string) error {
	// 1. Валідація підпису
	expectedSig := s.calculateSignature(data)
	if expectedSig != signature {
		s.l.Warnw("LiqPay webhook: invalid signature", "expected", expectedSig, "received", signature)
		return domain.ErrInvalidSignature
	}

	// 2. Декодування даних
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		s.l.Errorw("LiqPay webhook: failed to decode data", "error", err)
		return fmt.Errorf("failed to decode LiqPay data: %w", err)
	}

	var liqPayData domain.LiqPayData
	if err := json.Unmarshal(decoded, &liqPayData); err != nil {
		s.l.Errorw("LiqPay webhook: failed to parse data JSON", "error", err)
		return fmt.Errorf("failed to parse LiqPay data: %w", err)
	}

	s.l.Infow("LiqPay webhook received",
		"order_id", liqPayData.OrderID,
		"status", liqPayData.Status,
		"amount", liqPayData.Amount,
	)

	// 3. Пошук платежу
	orderID, err := uuid.Parse(liqPayData.OrderID)
	if err != nil {
		s.l.Errorw("LiqPay webhook: invalid order_id format", "order_id", liqPayData.OrderID)
		return fmt.Errorf("invalid order_id in webhook: %w", err)
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			// Автоматично створюємо платіж, якщо його ще немає в БД
			order, findErr := s.orderRepo.FindByID(ctx, orderID)
			if findErr != nil {
				s.l.Errorw("LiqPay webhook: order not found", "error", findErr, "order_id", orderID)
				return findErr
			}

			if createErr := s.CreatePayment(ctx, orderID, order.TotalPrice); createErr != nil {
				s.l.Errorw("LiqPay webhook: failed to create payment", "error", createErr, "order_id", orderID)
				return createErr
			}

			// Отримуємо створений запис
			payment, err = s.paymentRepo.FindByOrderID(ctx, orderID)
			if err != nil {
				return err
			}
		} else {
			s.l.Errorw("LiqPay webhook: payment lookup failed", "error", err, "order_id", orderID)
			return err
		}
	}

	// 4. Ідемпотентність: якщо payment вже в success — ігноруємо
	if payment.Status == "success" {
		s.l.Infow("LiqPay webhook: payment already processed (idempotent skip)",
			"payment_id", payment.ID,
			"order_id", orderID,
		)
		return nil
	}

	transactionID := fmt.Sprintf("%d", liqPayData.LiqPayID)

	// 5. Обробка за статусом
	switch liqPayData.Status {
	case "success", "sandbox":
		// Атомарна транзакція для оновлення статусів замовлення
		err = s.orderRepo.WithTransaction(ctx, func(txCtx context.Context, txRepo orderDomain.OrderRepository) error {
			order, err := txRepo.FindByID(txCtx, orderID)
			if err != nil {
				return err
			}

			switch order.StatusID {
			case orderDomain.StatusPendingPayment:
				// Перевірка та перехід PendingPayment -> Paid
				if err := orderDomain.ValidateTransition(order.StatusID, orderDomain.StatusPaid); err != nil {
					return err
				}

				// Записуємо історію PendingPayment -> Paid
				fromStatus := orderDomain.StatusPendingPayment
				if err := txRepo.CreateStatusHistory(txCtx, &orderDomain.OrderStatusHistory{
					OrderID:      orderID,
					FromStatusID: &fromStatus,
					ToStatusID:   orderDomain.StatusPaid,
					Source:       orderDomain.SourcePaymentWebhook,
				}); err != nil {
					return err
				}

				// Перевірка та перехід Paid -> Processing
				if err := orderDomain.ValidateTransition(orderDomain.StatusPaid, orderDomain.StatusProcessing); err != nil {
					return err
				}

				// Оновлюємо статус на Processing
				if err := txRepo.UpdateStatus(txCtx, orderID, orderDomain.StatusProcessing); err != nil {
					return err
				}

				// Записуємо історію Paid -> Processing
				paidStatus := orderDomain.StatusPaid
				if err := txRepo.CreateStatusHistory(txCtx, &orderDomain.OrderStatusHistory{
					OrderID:      orderID,
					FromStatusID: &paidStatus,
					ToStatusID:   orderDomain.StatusProcessing,
					Source:       orderDomain.SourcePaymentWebhook,
				}); err != nil {
					return err
				}

			case orderDomain.StatusProcessing, orderDomain.StatusShipped, orderDomain.StatusDelivered:
				s.l.Infow("LiqPay webhook: order already processed (idempotent skip)", "order_id", orderID)
				return nil
			case orderDomain.StatusCancelled, orderDomain.StatusRefunded:
				s.l.Warnw("LiqPay webhook: order is cancelled/refunded, ignoring payment", "order_id", orderID)
				return nil
			}
			return nil
		})
		if err != nil {
			s.l.Errorw("LiqPay webhook: failed to update order status", "error", err, "order_id", orderID)
			return err
		}

		// Оновлюємо payment статус ТІЛЬКИ якщо транзакція замовлення успішна.
		// Якщо транзакція впала - payment залишається "pending" і webhook можна буде повторити.
		if err := s.paymentRepo.UpdateStatus(ctx, payment.ID, "success", transactionID, ""); err != nil {
			return err
		}

		// Trigger Notifications (async)
		s.sendNotifications(ctx, orderID)

		s.l.Infow("LiqPay payment successful",
			"order_id", orderID,
			"transaction_id", transactionID,
		)

	case "failure", "error":
		errMsg := liqPayData.ErrDescription
		if errMsg == "" {
			errMsg = liqPayData.ErrCode
		}
		if err := s.paymentRepo.UpdateStatus(ctx, payment.ID, "failed", transactionID, errMsg); err != nil {
			return err
		}

		s.l.Warnw("LiqPay payment failed",
			"order_id", orderID,
			"err_code", liqPayData.ErrCode,
			"err_description", liqPayData.ErrDescription,
		)

	default:
		s.l.Infow("LiqPay webhook: unhandled status", "status", liqPayData.Status, "order_id", orderID)
	}

	return nil
}

// GeneratePaymentURL формує URL для оплати
func (s *paymentService) GeneratePaymentURL(orderID uuid.UUID, amount int, orderNumber int64, paytypes string) string {
	if s.cfg.Env == "development" || s.cfg.Env == "local" {
		// Формуємо лінк на мок-сторінку фронтенда
		return fmt.Sprintf("%s/ua/mock-payment?order_id=%s&result_url=%s/orders/%s/success",
			s.cfg.FrontendURL, orderID.String(), s.cfg.FrontendURL, orderID.String())
	}

	if s.cfg.LiqPayPublicKey == "" || s.cfg.LiqPayPrivateKey == "" {
		return ""
	}

	// Формуємо параметри для LiqPay
	params := map[string]interface{}{
		"public_key":  s.cfg.LiqPayPublicKey,
		"version":     3,
		"action":      "pay",
		"amount":      fmt.Sprintf("%.2f", float64(amount)/100), // копійки → гривні
		"currency":    "UAH",
		"description": fmt.Sprintf("Замовлення №%d — AquaWheel Store", orderNumber),
		"order_id":    orderID.String(),
		"result_url":  s.cfg.FrontendURL + "/orders/" + orderID.String() + "/success",
		"server_url":  s.cfg.APIHost + "/api/webhooks/liqpay",
	}

	if paytypes != "" {
		params["paytypes"] = paytypes
	}

	jsonData, err := json.Marshal(params)
	if err != nil {
		s.l.Errorw("failed to marshal LiqPay params", "error", err)
		return ""
	}

	data := base64.StdEncoding.EncodeToString(jsonData)
	signature := s.calculateSignature(data)

	return fmt.Sprintf("https://www.liqpay.ua/api/3/checkout?data=%s&signature=%s", data, signature)
}

// calculateSignature обчислює підпис LiqPay: base64(sha1(private_key + data + private_key))
func (s *paymentService) calculateSignature(data string) string {
	raw := s.cfg.LiqPayPrivateKey + data + s.cfg.LiqPayPrivateKey
	hash := sha1.Sum([]byte(raw))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// sendNotifications відправляє сповіщення після успішної оплати (async)
func (s *paymentService) sendNotifications(ctx context.Context, orderID uuid.UUID) {
	go func() {
		order, err := s.orderRepo.FindByID(ctx, orderID)
		if err != nil {
			s.l.Error("failed to load order for notifications", zap.Error(err), zap.String("order_id", orderID.String()))
			return
		}

		// Генеруємо менеджерський токен для secure доступу до замовлення
		var managerLink string
		plainToken, tokenHash, err := orderDomain.GenerateManagerAccessToken()
		if err != nil {
			s.l.Error("failed to generate manager token", zap.Error(err), zap.String("order_id", orderID.String()))
		} else {
			expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 днів
			if setErr := s.orderRepo.SetManagerToken(ctx, orderID, tokenHash, expiresAt); setErr != nil {
				s.l.Error("failed to save manager token", zap.Error(setErr), zap.String("order_id", orderID.String()))
			} else {
				managerLink = fmt.Sprintf("%s/manager/orders/%d?token=%s", s.cfg.ManagerBaseURL, order.OrderNumber, plainToken)
				s.l.Infow("Manager access token generated",
					"order_number", order.OrderNumber,
					"expires_at", expiresAt,
					"manager_link", managerLink,
				)
			}
		}

		// Формуємо дані для листа
		emailItems := make([]email.OrderItemEmail, len(order.Items))
		for i, item := range order.Items {
			emailItems[i] = email.OrderItemEmail{
				Name:     fmt.Sprintf("Variation %s", item.VariationID.String()[:8]),
				SKU:      "", // SKU можна підтягнути окремо якщо потрібно
				Quantity: item.Quantity,
				Price:    item.Price,
				Total:    item.TotalPrice,
			}
		}

		var deliveryData email.DeliveryEmail
		if order.Delivery != nil {
			deliveryData = email.DeliveryEmail{
				Provider:      order.Delivery.Provider,
				CityName:      order.Delivery.CityName,
				WarehouseName: order.Delivery.WarehouseName,
			}
		}

		emailData := email.OrderEmailData{
			OrderNumber:  order.OrderNumber,
			CustomerName: order.FirstName + " " + order.LastName,
			Email:        order.Email,
			Phone:        order.Phone,
			Items:        emailItems,
			TotalPrice:   order.TotalPrice,
			Delivery:     deliveryData,
			ManagerLink:  managerLink,
		}

		// Лист 1: Клієнту
		if err := s.emailProvider.SendOrderConfirmationEmail(order.Email, emailData); err != nil {
			s.l.Error("failed to send order confirmation email", zap.Error(err))
		}

		// Лист 2: Адміністратору (з manager link)
		if s.cfg.AdminNotificationEmail != "" {
			if err := s.emailProvider.SendAdminOrderNotification(s.cfg.AdminNotificationEmail, emailData); err != nil {
				s.l.Error("failed to send admin order notification", zap.Error(err))
			}
		}
	}()
}

// SimulatePayment імітує успішну оплату (тільки для розробки)
func (s *paymentService) SimulatePayment(ctx context.Context, orderID uuid.UUID) error {
	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			// Автоматично створюємо платіж, якщо його ще немає в БД
			order, findErr := s.orderRepo.FindByID(ctx, orderID)
			if findErr != nil {
				s.l.Errorw("SimulatePayment: order not found", "error", findErr, "order_id", orderID)
				return findErr
			}

			if createErr := s.CreatePayment(ctx, orderID, order.TotalPrice); createErr != nil {
				s.l.Errorw("SimulatePayment: failed to create payment", "error", createErr, "order_id", orderID)
				return createErr
			}

			// Отримуємо створений запис
			payment, err = s.paymentRepo.FindByOrderID(ctx, orderID)
			if err != nil {
				return err
			}
		} else {
			s.l.Errorw("SimulatePayment: payment lookup failed", "error", err, "order_id", orderID)
			return err
		}
	}

	if payment.Status == "success" {
		s.l.Infow("SimulatePayment: payment already processed", "order_id", orderID)
		return nil
	}

	transactionID := "mock_tx_" + time.Now().Format("20060102150405")

	// Атомарна транзакція для оновлення статусів замовлення
	err = s.orderRepo.WithTransaction(ctx, func(txCtx context.Context, txRepo orderDomain.OrderRepository) error {
		order, err := txRepo.FindByID(txCtx, orderID)
		if err != nil {
			return err
		}

		switch order.StatusID {
		case orderDomain.StatusPendingPayment:
			if err := orderDomain.ValidateTransition(order.StatusID, orderDomain.StatusPaid); err != nil {
				return err
			}

			fromStatus := orderDomain.StatusPendingPayment
			if err := txRepo.CreateStatusHistory(txCtx, &orderDomain.OrderStatusHistory{
				OrderID:      orderID,
				FromStatusID: &fromStatus,
				ToStatusID:   orderDomain.StatusPaid,
				Source:       orderDomain.SourceSystem,
				Comment:      ptr("Mock payment"),
			}); err != nil {
				return err
			}

			if err := orderDomain.ValidateTransition(orderDomain.StatusPaid, orderDomain.StatusProcessing); err != nil {
				return err
			}

			if err := txRepo.UpdateStatus(txCtx, orderID, orderDomain.StatusProcessing); err != nil {
				return err
			}

			paidStatus := orderDomain.StatusPaid
			if err := txRepo.CreateStatusHistory(txCtx, &orderDomain.OrderStatusHistory{
				OrderID:      orderID,
				FromStatusID: &paidStatus,
				ToStatusID:   orderDomain.StatusProcessing,
				Source:       orderDomain.SourceSystem,
				Comment:      ptr("Mock payment"),
			}); err != nil {
				return err
			}

		case orderDomain.StatusProcessing, orderDomain.StatusShipped, orderDomain.StatusDelivered:
			s.l.Infow("SimulatePayment: order already processed (idempotent skip)", "order_id", orderID)
			return nil
		case orderDomain.StatusCancelled, orderDomain.StatusRefunded:
			s.l.Warnw("SimulatePayment: order is cancelled/refunded, ignoring payment", "order_id", orderID)
			return nil
		}
		return nil
	})
	if err != nil {
		s.l.Errorw("failed to update order status after mock payment", "error", err, "order_id", orderID)
		return err
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.ID, "success", transactionID, ""); err != nil {
		return err
	}

	s.sendNotifications(ctx, orderID)

	s.l.Infow("Mock payment successful",
		"order_id", orderID,
		"transaction_id", transactionID,
	)

	return nil
}

// ProcessRefundStub - заглушка для обробки повернення коштів
func (s *paymentService) ProcessRefundStub(ctx context.Context, orderID uuid.UUID) error {
	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		s.l.Errorw("ProcessRefundStub: payment lookup failed", "error", err, "order_id", orderID)
		return err
	}

	if payment.Status != "success" {
		s.l.Warnw("ProcessRefundStub: payment is not in success status", "order_id", orderID, "status", payment.Status)
		// We could still return nil or an error, but let's assume it's okay for now.
	}

	s.l.Infow("Refund processing stub called. In a real scenario, this would call LiqPay API to refund the transaction.",
		"order_id", orderID,
		"transaction_id", payment.TransactionID,
		"amount", payment.Amount,
	)

	// Update payment status to refunded
	if err := s.paymentRepo.UpdateStatus(ctx, payment.ID, "refunded", payment.TransactionID, "Refund processed via stub"); err != nil {
		return err
	}

	return nil
}

func ptr(s string) *string {
	return &s
}

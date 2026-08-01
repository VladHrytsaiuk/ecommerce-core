package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/payment/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"gorm.io/gorm"
)

type paymentRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewPaymentRepository створює новий інстанс репозиторію платежів
func NewPaymentRepository(db *gorm.DB, l logger.Logger) domain.PaymentRepository {
	return &paymentRepository{db: db, l: l}
}

// Create зберігає новий запис Payment
func (r *paymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	if err := r.db.WithContext(ctx).Create(payment).Error; err != nil {
		r.l.Errorw("failed to create payment", "error", err, "order_id", payment.OrderID)
		return err
	}
	return nil
}

// FindByOrderID знаходить платіж за order_id
func (r *paymentRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	var payment domain.Payment
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		First(&payment).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrPaymentNotFound
		}
		r.l.Errorw("failed to find payment by order_id", "error", err, "order_id", orderID)
		return nil, err
	}
	return &payment, nil
}

// UpdateStatus оновлює статус платежу
func (r *paymentRepository) UpdateStatus(ctx context.Context, paymentID uuid.UUID, status, transactionID, errorMessage string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if transactionID != "" {
		updates["transaction_id"] = transactionID
	}
	if errorMessage != "" {
		updates["error_message"] = errorMessage
	}

	result := r.db.WithContext(ctx).
		Model(&domain.Payment{}).
		Where("id = ?", paymentID).
		Updates(updates)
	if result.Error != nil {
		r.l.Errorw("failed to update payment status", "error", result.Error, "payment_id", paymentID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrPaymentNotFound
	}
	return nil
}

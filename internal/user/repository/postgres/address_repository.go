package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type addressRepository struct {
	db     *gorm.DB
	logger logger.Logger
}

// NewUserAddressRepository створює новий екземпляр GORM-репозиторію для адрес користувачів.
func NewUserAddressRepository(db *gorm.DB, l logger.Logger) domain.UserAddressRepository {
	return &addressRepository{
		db:     db,
		logger: l,
	}
}

// Create зберігає нову адресу доставки для користувача.
func (r *addressRepository) Create(ctx context.Context, address *domain.UserAddress) error {
	err := r.db.WithContext(ctx).Create(address).Error
	if err != nil {
		r.logger.Error("failed to create address",
			zap.Error(err),
			zap.String("user_id", address.UserID.String()),
		)
		return fmt.Errorf("failed to create address: %w", err)
	}
	return nil
}

// FindAllByUserID повертає всі активні адреси конкретного користувача.
func (r *addressRepository) FindAllByUserID(ctx context.Context, userID uuid.UUID) ([]domain.UserAddress, error) {
	var addresses []domain.UserAddress
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&addresses).Error
	if err != nil {
		return nil, fmt.Errorf("failed to find all addresses by user id: %w", err)
	}
	return addresses, nil
}

// FindByID шукає одну адресу за її унікальним ID.
func (r *addressRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.UserAddress, error) {
	var address domain.UserAddress
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&address).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("address not found: %w", err)
		}
		return nil, fmt.Errorf("failed to find address by id: %w", err)
	}
	return &address, nil
}

// Update оновлює поля існуючої адреси.
func (r *addressRepository) Update(ctx context.Context, address *domain.UserAddress) error {
	err := r.db.WithContext(ctx).Save(address).Error
	if err != nil {
		return fmt.Errorf("failed to update address: %w", err)
	}
	return nil
}

// Delete виконує "м'яке видалення" (Soft Delete) адреси.
func (r *addressRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// GORM автоматично використовує Soft Delete, оскільки в структурі є DeletedAt
	err := r.db.WithContext(ctx).Delete(&domain.UserAddress{}, id).Error
	if err != nil {
		return fmt.Errorf("failed to delete address: %w", err)
	}
	return nil
}

// UnsetDefaultAllForUser робить всі адреси користувача "не-дефолтними".
// Зазвичай використовується перед тим, як зробити якусь одну адресу основною.
func (r *addressRepository) UnsetDefaultAllForUser(ctx context.Context, userID uuid.UUID) error {
	// Скидання IsDefault для усіх адрес юзера
	err := r.db.WithContext(ctx).
		Model(&domain.UserAddress{}).
		Where("user_id = ?", userID).
		Update("is_default", false).Error

	if err != nil {
		return fmt.Errorf("failed to unset default addresses for user: %w", err)
	}
	return nil
}

// Atomic виконує кілька операцій в одній транзакції.
func (r *addressRepository) Atomic(ctx context.Context, fn func(domain.UserAddressRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Створюємо тимчасовий репозиторій, що працює через об'єкт транзакції (tx)
		txRepo := &addressRepository{
			db:     tx,
			logger: r.logger,
		}
		return fn(txRepo)
	})
}

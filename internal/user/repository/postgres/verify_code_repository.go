package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"gorm.io/gorm"
)

type verifyCodeRepository struct {
	db     *gorm.DB
	logger logger.Logger
}

// NewVerifyCodeRepository створює новий репозиторій для кодів верифікації.
func NewVerifyCodeRepository(db *gorm.DB, l logger.Logger) domain.VerifyCodeRepository {
	return &verifyCodeRepository{
		db:     db,
		logger: l,
	}
}

func (r *verifyCodeRepository) getDB(ctx context.Context) *gorm.DB {
	return db.GetTx(ctx, r.db).WithContext(ctx)
}

// Create зберігає новий код у базі.
func (r *verifyCodeRepository) Create(ctx context.Context, code *domain.VerifyCode) error {
	if err := r.getDB(ctx).Create(code).Error; err != nil {
		return fmt.Errorf("failed to create verify code: %w", err)
	}
	return nil
}

// FindLastByTarget знаходить останній створений код для вказаної цілі (напр., email) та типу.
func (r *verifyCodeRepository) FindLastByTarget(ctx context.Context, target string, codeType string) (*domain.VerifyCode, error) {
	var code domain.VerifyCode
	err := r.getDB(ctx).
		Where("target = ? AND type = ?", target, codeType).
		Order("created_at DESC").
		First(&code).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Або можна повертати кастомну помилку, але nil зручніше для перевірки
		}
		return nil, fmt.Errorf("failed to find verify code: %w", err)
	}

	return &code, nil
}

// FindByCode знаходить код за його значенням та типом.
func (r *verifyCodeRepository) FindByCode(ctx context.Context, codeValue string, codeType string) (*domain.VerifyCode, error) {
	var code domain.VerifyCode
	err := r.getDB(ctx).
		Where("code = ? AND type = ?", codeValue, codeType).
		First(&code).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find verify code by code value: %w", err)
	}

	return &code, nil
}

// MarkAsUsed позначає код як використаний.
func (r *verifyCodeRepository) MarkAsUsed(ctx context.Context, id uuid.UUID) error {
	err := r.getDB(ctx).
		Model(&domain.VerifyCode{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_used": true,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to mark code as used: %w", err)
	}
	return nil
}

// UpdateAttempts оновлює кількість спроб перевірки коду.
func (r *verifyCodeRepository) UpdateAttempts(ctx context.Context, id uuid.UUID, attempts int) error {
	err := r.getDB(ctx).
		Model(&domain.VerifyCode{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"attempts": attempts,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to update verify code attempts: %w", err)
	}
	return nil
}

// DeleteExpired видаляє всі прострочені коди верифікації.
func (r *verifyCodeRepository) DeleteExpired(ctx context.Context) error {
	err := r.getDB(ctx).Where("expires_at < NOW()").Delete(&domain.VerifyCode{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete expired verification codes: %w", err)
	}
	return nil
}

package postgres

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shipment/domain"
	"gorm.io/gorm"
)

type shippingRuleRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewShippingRuleRepository створює новий інстанс репозиторію для правил доставки
func NewShippingRuleRepository(db *gorm.DB, l logger.Logger) domain.ShippingRuleRepository {
	return &shippingRuleRepository{db: db, l: l}
}

// GetActiveRules повертає всі активні правила безкоштовної доставки
func (r *shippingRuleRepository) GetActiveRules(ctx context.Context) ([]domain.ShippingRule, error) {
	var rules []domain.ShippingRule
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Order("provider ASC").
		Find(&rules).Error
	if err != nil {
		r.l.Errorw("failed to get active shipping rules", "error", err)
		return nil, err
	}
	return rules, nil
}

// UpdateShippingRule оновлює або створює правило доставки
func (r *shippingRuleRepository) UpdateShippingRule(ctx context.Context, provider string, minOrderAmount int) error {
	var rule domain.ShippingRule
	err := r.db.WithContext(ctx).Where("provider = ?", provider).First(&rule).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new
			rule = domain.ShippingRule{
				Provider:       provider,
				MinOrderAmount: minOrderAmount,
				IsActive:       true,
			}
			return r.db.WithContext(ctx).Create(&rule).Error
		}
		r.l.Errorw("failed to fetch shipping rule for update", "error", err, "provider", provider)
		return err
	}

	// Update existing
	rule.MinOrderAmount = minOrderAmount
	rule.IsActive = true
	return r.db.WithContext(ctx).Save(&rule).Error
}

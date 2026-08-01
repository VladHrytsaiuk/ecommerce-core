package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/redirect/domain"
	"gorm.io/gorm"
)

type redirectRepository struct {
	db *gorm.DB
}

func NewRedirectRepository(db *gorm.DB) domain.RedirectRepository {
	return &redirectRepository{db: db}
}

func (r *redirectRepository) SaveHistory(ctx context.Context, history *domain.UrlHistory) error {
	err := r.db.WithContext(ctx).Create(history).Error
	if err != nil {
		return fmt.Errorf("failed to save url history: %w", err)
	}
	return nil
}

func (r *redirectRepository) FindByOldSlug(ctx context.Context, oldSlug string) (*domain.UrlHistory, error) {
	var history domain.UrlHistory
	err := r.db.WithContext(ctx).
		Where("old_slug = ?", oldSlug).
		Order("created_at DESC").
		First(&history).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil if not found
		}
		return nil, fmt.Errorf("failed to find url history: %w", err)
	}
	return &history, nil
}

func (r *redirectRepository) FindCurrentSlug(ctx context.Context, entityType string, entityID uuid.UUID, lang string) (string, error) {
	var currentSlug string
	var err error

	switch entityType {
	case "product":
		err = r.db.WithContext(ctx).
			Table("product_translation").
			Joins("JOIN product ON product.id = product_translation.product_id AND product.deleted_at IS NULL").
			Select("product_translation.slug").
			Where("product_translation.product_id = ? AND product_translation.language_code = ?", entityID, lang).
			Limit(1).
			Scan(&currentSlug).Error
	case "category":
		err = r.db.WithContext(ctx).
			Table("category_translation").
			Joins("JOIN category ON category.id = category_translation.category_id AND category.deleted_at IS NULL").
			Select("category_translation.slug").
			Where("category_translation.category_id = ? AND category_translation.language_code = ?", entityID, lang).
			Limit(1).
			Scan(&currentSlug).Error
	case "product_variation":
		err = r.db.WithContext(ctx).
			Table("product_variation").
			Joins("JOIN product ON product.id = product_variation.product_id AND product.deleted_at IS NULL").
			Select("product_variation.slug").
			Where("product_variation.id = ? AND product_variation.deleted_at IS NULL", entityID).
			Limit(1).
			Scan(&currentSlug).Error
	default:
		return "", fmt.Errorf("unsupported entity type: %s", entityType)
	}

	if err != nil {
		return "", err
	}

	return currentSlug, nil
}

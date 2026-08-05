//go:build legacy && ignore
// +build legacy,ignore

package postgres

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type wishlistRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewWishlistRepository створює новий інстанс репозиторію
func NewWishlistRepository(db *gorm.DB, l logger.Logger) domain.WishlistRepository {
	return &wishlistRepository{db: db, l: l}
}

// preloadVariations повертає базовий запит з усіма Preload для ProductVariation (формат Brief)
func (r *wishlistRepository) preloadVariations(ctx context.Context, lang string) *gorm.DB {
	return r.db.WithContext(ctx).
		Preload("Unit").
		Preload("Product", func(db *gorm.DB) *gorm.DB {
			return db.
				Preload("Translations", "language_code = ?", lang).
				Preload("Brand").
				Preload("BundleItems").
				Preload("Images", func(db *gorm.DB) *gorm.DB {
					return db.Order("sort_order asc")
				})
		})
}

func (r *wishlistRepository) GetByUserID(ctx context.Context, userID uuid.UUID, lang string) ([]productDomain.ProductVariation, error) {
	var items []domain.WishlistItem

	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&items).Error
	if err != nil {
		r.l.Errorw("failed to get wishlist items by user_id", "error", err, "user_id", userID)
		return nil, err
	}

	return r.loadVariations(ctx, items, lang)
}

func (r *wishlistRepository) GetBySessionID(ctx context.Context, sessionID string, lang string) ([]productDomain.ProductVariation, error) {
	var items []domain.WishlistItem

	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Find(&items).Error
	if err != nil {
		r.l.Errorw("failed to get wishlist items by session_id", "error", err, "session_id", sessionID)
		return nil, err
	}

	return r.loadVariations(ctx, items, lang)
}

// loadVariations завантажує варіації з усіма зв'язками на основі wishlist items
func (r *wishlistRepository) loadVariations(ctx context.Context, items []domain.WishlistItem, lang string) ([]productDomain.ProductVariation, error) {
	if len(items) == 0 {
		return []productDomain.ProductVariation{}, nil
	}

	variationIDs := make([]uuid.UUID, len(items))
	for i, item := range items {
		variationIDs[i] = item.VariationID
	}

	var variations []productDomain.ProductVariation
	err := r.preloadVariations(ctx, lang).
		Where("product_variation.id IN ?", variationIDs).
		Where("product_variation.is_active = ?", true).
		Find(&variations).Error
	if err != nil {
		r.l.Errorw("failed to load wishlist variations", "error", err)
		return nil, err
	}

	// Зберігаємо порядок з wishlist (за created_at DESC)
	orderedVariations := make([]productDomain.ProductVariation, 0, len(variations))
	variationMap := make(map[uuid.UUID]productDomain.ProductVariation, len(variations))
	for _, v := range variations {
		variationMap[v.ID] = v
	}
	for _, item := range items {
		if v, ok := variationMap[item.VariationID]; ok {
			orderedVariations = append(orderedVariations, v)
		}
	}

	return orderedVariations, nil
}

func (r *wishlistRepository) Add(ctx context.Context, item *domain.WishlistItem) error {
	// ON CONFLICT DO NOTHING — якщо вже є, просто ігноруємо
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(item).Error
	if err != nil {
		r.l.Errorw("failed to add wishlist item", "error", err, "variation_id", item.VariationID)
		return err
	}
	return nil
}

func (r *wishlistRepository) Remove(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	query := r.db.WithContext(ctx).Where("variation_id = ?", variationID)

	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	} else if sessionID != nil {
		query = query.Where("session_id = ?", *sessionID)
	} else {
		return domain.ErrNoIdentifier
	}

	result := query.Delete(&domain.WishlistItem{})
	if result.Error != nil {
		r.l.Errorw("failed to remove wishlist item", "error", result.Error, "variation_id", variationID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrItemNotInWishlist
	}
	return nil
}

func (r *wishlistRepository) SyncSessionToUser(ctx context.Context, sessionID string, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Отримуємо всі items з анонімної сесії
		var sessionItems []domain.WishlistItem
		if err := tx.Where("session_id = ?", sessionID).Find(&sessionItems).Error; err != nil {
			r.l.Errorw("sync: failed to fetch session items", "error", err, "session_id", sessionID)
			return err
		}

		if len(sessionItems) == 0 {
			return nil // Нічого синхронізувати
		}

		// 2. Вставляємо кожен item для user (ON CONFLICT DO NOTHING для дедуплікації)
		for _, item := range sessionItems {
			newItem := domain.WishlistItem{
				UserID:      &userID,
				VariationID: item.VariationID,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&newItem).Error; err != nil {
				r.l.Errorw("sync: failed to insert user item", "error", err, "variation_id", item.VariationID)
				return err
			}
		}

		// 3. Видаляємо всі items з анонімної сесії
		if err := tx.Where("session_id = ?", sessionID).Delete(&domain.WishlistItem{}).Error; err != nil {
			r.l.Errorw("sync: failed to cleanup session items", "error", err, "session_id", sessionID)
			return err
		}

		r.l.Infow("Wishlist sync completed", "session_id", sessionID, "user_id", userID, "items_synced", len(sessionItems))
		return nil
	})
}

func (r *wishlistRepository) DeleteExpiredAnonymous(ctx context.Context, olderThan time.Time) error {
	result := r.db.WithContext(ctx).
		Where("session_id IS NOT NULL AND created_at < ?", olderThan).
		Delete(&domain.WishlistItem{})
	if result.Error != nil {
		r.l.Errorw("failed to delete expired anonymous wishlist items", "error", result.Error)
		return result.Error
	}
	if result.RowsAffected > 0 {
		r.l.Infow("Cleaned up expired anonymous wishlist items", "deleted_count", result.RowsAffected)
	}
	return nil
}

func (r *wishlistRepository) VariationExists(ctx context.Context, variationID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&productDomain.ProductVariation{}).
		Where("id = ? AND is_active = ?", variationID, true).
		Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check variation existence", "error", err, "variation_id", variationID)
		return false, err
	}
	return count > 0, nil
}

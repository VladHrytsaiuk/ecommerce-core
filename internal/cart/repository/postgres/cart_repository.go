//go:build legacy && ignore
// +build legacy,ignore

package postgres

import (
	"context"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cartRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewCartRepository створює новий інстанс репозиторію
func NewCartRepository(db *gorm.DB, l logger.Logger) domain.CartRepository {
	return &cartRepository{db: db, l: l}
}

// preloadVariations повертає базовий запит з усіма Preload для ProductVariation
func (r *cartRepository) preloadVariations(ctx context.Context, lang string) *gorm.DB {
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

// findCartByUserID знаходить кошик за user_id
func (r *cartRepository) findCartByUserID(ctx context.Context, userID uuid.UUID) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		First(&cart).Error
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

// findCartBySessionID знаходить кошик за session_id
func (r *cartRepository) findCartBySessionID(ctx context.Context, sessionID string) (*domain.Cart, error) {
	var cart domain.Cart
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&cart).Error
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

// findOrCreateCart знаходить або створює кошик для юзера/сесії
func (r *cartRepository) findOrCreateCart(ctx context.Context, tx *gorm.DB, userID *uuid.UUID, sessionID *string) (*domain.Cart, error) {
	var cart domain.Cart

	query := tx.WithContext(ctx)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	} else if sessionID != nil {
		query = query.Where("session_id = ?", *sessionID)
	} else {
		return nil, domain.ErrNoIdentifier
	}

	err := query.First(&cart).Error
	if err == nil {
		return &cart, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Кошик не знайдений — створюємо новий
	cart = domain.Cart{
		UserID:    userID,
		SessionID: sessionID,
	}
	if err := tx.WithContext(ctx).Create(&cart).Error; err != nil {
		return nil, err
	}
	return &cart, nil
}

// findCart знаходить кошик за user_id або session_id без створення нового.
// Повертає gorm.ErrRecordNotFound, якщо кошик не існує.
func (r *cartRepository) findCart(ctx context.Context, tx *gorm.DB, userID *uuid.UUID, sessionID *string) (*domain.Cart, error) {
	var cart domain.Cart
	query := tx.WithContext(ctx)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	} else if sessionID != nil {
		query = query.Where("session_id = ?", *sessionID)
	} else {
		return nil, domain.ErrNoIdentifier
	}
	err := query.First(&cart).Error
	if err != nil {
		return nil, err
	}
	return &cart, nil
}

func (r *cartRepository) GetByUserID(ctx context.Context, userID uuid.UUID, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
	cart, err := r.findCartByUserID(ctx, userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, []productDomain.ProductVariation{}, nil
		}
		r.l.Errorw("failed to get cart by user_id", "error", err, "user_id", userID)
		return nil, nil, err
	}
	return r.loadCartItems(ctx, cart, lang)
}

func (r *cartRepository) GetBySessionID(ctx context.Context, sessionID string, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
	cart, err := r.findCartBySessionID(ctx, sessionID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, []productDomain.ProductVariation{}, nil
		}
		r.l.Errorw("failed to get cart by session_id", "error", err, "session_id", sessionID)
		return nil, nil, err
	}
	return r.loadCartItems(ctx, cart, lang)
}

// loadCartItems завантажує items кошика разом з варіаціями
func (r *cartRepository) loadCartItems(ctx context.Context, cart *domain.Cart, lang string) (*domain.Cart, []productDomain.ProductVariation, error) {
	// Завантажуємо cart_items для цього кошика
	var items []domain.CartItem
	err := r.db.WithContext(ctx).
		Where("cart_id = ?", cart.ID).
		Order("created_at DESC").
		Find(&items).Error
	if err != nil {
		r.l.Errorw("failed to load cart items", "error", err, "cart_id", cart.ID)
		return nil, nil, err
	}

	cart.Items = items

	if len(items) == 0 {
		return cart, []productDomain.ProductVariation{}, nil
	}

	// Збираємо variation_ids
	variationIDs := make([]uuid.UUID, len(items))
	for i, item := range items {
		variationIDs[i] = item.VariationID
	}

	// Завантажуємо варіації з усіма зв'язками
	var variations []productDomain.ProductVariation
	err = r.preloadVariations(ctx, lang).
		Where("product_variation.id IN ?", variationIDs).
		Find(&variations).Error
	if err != nil {
		r.l.Errorw("failed to load cart variations", "error", err)
		return nil, nil, err
	}

	// Зберігаємо порядок з cart_items (за created_at DESC)
	orderedVariations := make([]productDomain.ProductVariation, 0, len(variations))
	variationMap := make(map[uuid.UUID]productDomain.ProductVariation, len(variations))
	for _, v := range variations {
		variationMap[v.ID] = v
	}
	var activeItems []domain.CartItem
	for _, item := range items {
		if v, ok := variationMap[item.VariationID]; ok {
			orderedVariations = append(orderedVariations, v)
			activeItems = append(activeItems, item)
		}
	}
	cart.Items = activeItems

	return cart, orderedVariations, nil
}

func (r *cartRepository) AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, err := r.findOrCreateCart(ctx, tx, userID, sessionID)
		if err != nil {
			r.l.Errorw("failed to find or create cart", "error", err)
			return err
		}

		// Upsert: якщо item вже є — збільшуємо quantity, якщо немає — створюємо
		item := domain.CartItem{
			CartID:      cart.ID,
			VariationID: variationID,
			Quantity:    quantity,
		}

		err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "cart_id"}, {Name: "variation_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"quantity":   gorm.Expr("cart_item.quantity + ?", quantity),
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}),
		}).Create(&item).Error
		if err != nil {
			r.l.Errorw("failed to add cart item", "error", err, "variation_id", variationID)
			return err
		}

		// Оновлюємо updated_at кошика
		return tx.Model(&domain.Cart{}).Where("id = ?", cart.ID).
			Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
	})
}

func (r *cartRepository) UpdateQuantity(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, err := r.findOrCreateCart(ctx, tx, userID, sessionID)
		if err != nil {
			return err
		}

		result := tx.Model(&domain.CartItem{}).
			Where("cart_id = ? AND variation_id = ?", cart.ID, variationID).
			Updates(map[string]interface{}{
				"quantity":   quantity,
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			})
		if result.Error != nil {
			r.l.Errorw("failed to update cart item quantity", "error", result.Error, "variation_id", variationID)
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrCartItemNotFound
		}

		// Оновлюємо updated_at кошика
		return tx.Model(&domain.Cart{}).Where("id = ?", cart.ID).
			Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
	})
}

func (r *cartRepository) RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, err := r.findCart(ctx, tx, userID, sessionID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				if variationID == uuid.Nil {
					return nil // Кошика не існує, очищення успішне
				}
				return domain.ErrCartItemNotFound
			}
			return err
		}

		if variationID == uuid.Nil {
			// Якщо передано uuid.Nil, очищуємо весь кошик (видаляємо всі товари та скидаємо промокод)
			if err := tx.Where("cart_id = ?", cart.ID).Delete(&domain.CartItem{}).Error; err != nil {
				r.l.Errorw("failed to clear cart items", "error", err, "cart_id", cart.ID)
				return err
			}

			if err := tx.Model(&domain.Cart{}).Where("id = ?", cart.ID).Update("promo_code_id", nil).Error; err != nil {
				r.l.Errorw("failed to reset cart promo code", "error", err, "cart_id", cart.ID)
				return err
			}
		} else {
			result := tx.Where("cart_id = ? AND variation_id = ?", cart.ID, variationID).
				Delete(&domain.CartItem{})
			if result.Error != nil {
				r.l.Errorw("failed to remove cart item", "error", result.Error, "variation_id", variationID)
				return result.Error
			}
			if result.RowsAffected == 0 {
				return domain.ErrCartItemNotFound
			}
		}

		// Оновлюємо updated_at кошика
		return tx.Model(&domain.Cart{}).Where("id = ?", cart.ID).
			Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
	})
}

func (r *cartRepository) SyncSessionToUser(ctx context.Context, sessionID string, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Знаходимо анонімний кошик
		var sessionCart domain.Cart
		err := tx.Where("session_id = ?", sessionID).First(&sessionCart).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil // Нічого синхронізувати
			}
			r.l.Errorw("sync: failed to fetch session cart", "error", err, "session_id", sessionID)
			return err
		}

		// 2. Отримуємо items з анонімного кошика
		var sessionItems []domain.CartItem
		if err := tx.Where("cart_id = ?", sessionCart.ID).Find(&sessionItems).Error; err != nil {
			r.l.Errorw("sync: failed to fetch session cart items", "error", err, "session_id", sessionID)
			return err
		}

		if len(sessionItems) == 0 {
			// Видаляємо порожній анонімний кошик
			return tx.Delete(&sessionCart).Error
		}

		// 3. Знаходимо або створюємо кошик юзера
		userCart, err := r.findOrCreateCart(ctx, tx, &userID, nil)
		if err != nil {
			r.l.Errorw("sync: failed to find or create user cart", "error", err, "user_id", userID)
			return err
		}

		// 4. Переносимо кожен item (з додаванням кількостей при конфлікті)
		for _, item := range sessionItems {
			newItem := domain.CartItem{
				CartID:      userCart.ID,
				VariationID: item.VariationID,
				Quantity:    item.Quantity,
			}
			err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "cart_id"}, {Name: "variation_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"quantity":   gorm.Expr("cart_item.quantity + ?", item.Quantity),
					"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
				}),
			}).Create(&newItem).Error
			if err != nil {
				r.l.Errorw("sync: failed to insert user cart item", "error", err, "variation_id", item.VariationID)
				return err
			}
		}

		// 5. Видаляємо items та сам анонімний кошик
		if err := tx.Where("cart_id = ?", sessionCart.ID).Delete(&domain.CartItem{}).Error; err != nil {
			r.l.Errorw("sync: failed to cleanup session cart items", "error", err, "session_id", sessionID)
			return err
		}
		if err := tx.Delete(&sessionCart).Error; err != nil {
			r.l.Errorw("sync: failed to delete session cart", "error", err, "session_id", sessionID)
			return err
		}

		r.l.Infow("Cart sync completed", "session_id", sessionID, "user_id", userID, "items_synced", len(sessionItems))
		return nil
	})
}

func (r *cartRepository) DeleteExpiredAnonymous(ctx context.Context, olderThan time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Знаходимо всі застарілі анонімні кошики
		var expiredCarts []domain.Cart
		err := tx.Where("session_id IS NOT NULL AND updated_at < ?", olderThan).Find(&expiredCarts).Error
		if err != nil {
			r.l.Errorw("failed to find expired anonymous carts", "error", err)
			return err
		}

		if len(expiredCarts) == 0 {
			return nil
		}

		cartIDs := make([]uuid.UUID, len(expiredCarts))
		for i, c := range expiredCarts {
			cartIDs[i] = c.ID
		}

		// Видаляємо items
		if err := tx.Where("cart_id IN ?", cartIDs).Delete(&domain.CartItem{}).Error; err != nil {
			r.l.Errorw("failed to delete expired cart items", "error", err)
			return err
		}

		// Видаляємо кошики
		result := tx.Where("id IN ?", cartIDs).Delete(&domain.Cart{})
		if result.Error != nil {
			r.l.Errorw("failed to delete expired anonymous carts", "error", result.Error)
			return result.Error
		}

		if result.RowsAffected > 0 {
			r.l.Infow("Cleaned up expired anonymous carts", "deleted_count", result.RowsAffected)
		}
		return nil
	})
}

func (r *cartRepository) VariationExists(ctx context.Context, variationID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&productDomain.ProductVariation{}).
		Joins("JOIN product ON product.id = product_variation.product_id AND product.deleted_at IS NULL").
		Where("product_variation.id = ? AND product_variation.is_active = ? AND product.is_active = ?", variationID, true, true).
		// Для наборів: перевіряємо, що всі компоненти доступні
		Where(`(
			product.is_bundle = false
			OR NOT EXISTS (
				SELECT 1 FROM product_bundle_item pbi
				JOIN product_variation pv_comp ON pv_comp.id = pbi.variation_id
				JOIN product p_comp ON p_comp.id = pv_comp.product_id
				WHERE pbi.bundle_id = product.id
				  AND (pv_comp.is_active = false OR pv_comp.deleted_at IS NOT NULL OR p_comp.is_active = false OR p_comp.deleted_at IS NOT NULL)
			)
		)`).
		Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check variation existence", "error", err, "variation_id", variationID)
		return false, err
	}
	return count > 0, nil
}

func (r *cartRepository) UpdatePromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string, promoCodeID *uuid.UUID) error {
	query := r.db.WithContext(ctx).Model(&domain.Cart{})
	if userID != nil {
		query = query.Where("user_id = ?", userID)
	} else if sessionID != nil {
		query = query.Where("session_id = ?", sessionID)
	} else {
		return domain.ErrNoIdentifier
	}
	return query.Update("promo_code_id", promoCodeID).Error
}

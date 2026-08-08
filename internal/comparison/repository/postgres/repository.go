// Package postgres implements Comparison persistence without importing Catalog
// repositories. It queries only stable Core catalog tables it references.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/comparison/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) List(ctx context.Context, owner domain.Owner) ([]domain.List, error) {
	if !owner.Valid() {
		return nil, domain.ErrInvalidOwner
	}
	var lists []listRecord
	query := repository.db.WithContext(ctx).Order("updated_at DESC")
	if owner.UserID != nil {
		query = query.Where("user_id = ?", *owner.UserID)
	} else {
		query = query.Where("session_id = ?", *owner.SessionID)
	}
	if err := query.Find(&lists).Error; err != nil {
		return nil, err
	}
	result := make([]domain.List, 0, len(lists))
	for _, list := range lists {
		var items []itemRecord
		if err := repository.db.WithContext(ctx).Where("comparison_list_id = ?", list.ID).Order("created_at DESC").Find(&items).Error; err != nil {
			return nil, err
		}
		mapped := make([]domain.Item, 0, len(items))
		for _, item := range items {
			mapped = append(mapped, domain.Item{ID: item.ID, ProductVariantID: item.ProductVariantID, CreatedAt: item.CreatedAt})
		}
		result = append(result, domain.List{ID: list.ID, CategoryID: list.CategoryID, Items: mapped})
	}
	return result, nil
}

func (repository *Repository) Add(ctx context.Context, owner domain.Owner, variantID uuid.UUID, maxItems int) error {
	if !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	if maxItems < 1 {
		return domain.ErrComparisonAtLimit
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		categoryID, err := productCategory(ctx, tx, variantID)
		if err != nil {
			return err
		}
		listID, err := ensureList(ctx, tx, owner, categoryID)
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.Raw(`SELECT EXISTS (SELECT 1 FROM comparison_items WHERE comparison_list_id = ? AND product_variant_id = ?)`, listID, variantID).Scan(&exists).Error; err != nil {
			return err
		}
		if exists {
			return nil
		}
		var count int64
		if err := tx.Model(&itemRecord{}).Where("comparison_list_id = ?", listID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(maxItems) {
			return domain.ErrComparisonAtLimit
		}
		if err := tx.Create(&itemRecord{ID: uuid.New(), ComparisonListID: listID, ProductVariantID: variantID}).Error; err != nil {
			return err
		}
		return tx.Model(&listRecord{}).Where("id = ?", listID).Update("updated_at", time.Now().UTC()).Error
	})
}

func (repository *Repository) Remove(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var listIDs []uuid.UUID
		query := tx.Model(&listRecord{})
		if owner.UserID != nil {
			query = query.Where("user_id = ?", *owner.UserID)
		} else {
			query = query.Where("session_id = ?", *owner.SessionID)
		}
		if err := query.Pluck("id", &listIDs).Error; err != nil {
			return err
		}
		if len(listIDs) == 0 {
			return nil
		}
		if err := tx.Where("comparison_list_id IN ? AND product_variant_id = ?", listIDs, variantID).Delete(&itemRecord{}).Error; err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM comparison_lists lists WHERE lists.id IN ? AND NOT EXISTS (SELECT 1 FROM comparison_items items WHERE items.comparison_list_id = lists.id)`, listIDs).Error
	})
}

// MergeGuestComparison keeps category groups intact. The explicit policy is
// newest-first: after deduplication, only the newest maxItems across the user
// and guest lists survive. This makes capacity enforcement deterministic and
// never silently privileges the pre-login list.
func (repository *Repository) MergeGuestComparison(ctx context.Context, userID, sessionID uuid.UUID, maxItems int) error {
	if userID == uuid.Nil || sessionID == uuid.Nil {
		return domain.ErrInvalidOwner
	}
	if maxItems < 1 {
		return domain.ErrComparisonAtLimit
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var guestLists []listRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("session_id = ?", sessionID).Find(&guestLists).Error; err != nil {
			return err
		}
		for _, guest := range guestLists {
			userListID, err := ensureList(ctx, tx, domain.Owner{UserID: &userID}, guest.CategoryID)
			if err != nil {
				return err
			}
			var candidates []itemRecord
			if err := tx.Where("comparison_list_id IN ?", []uuid.UUID{userListID, guest.ID}).Order("created_at DESC, id DESC").Find(&candidates).Error; err != nil {
				return err
			}
			kept := newestUniqueItems(candidates, maxItems)
			if err := tx.Where("comparison_list_id = ?", userListID).Delete(&itemRecord{}).Error; err != nil {
				return err
			}
			if len(kept) > 0 {
				items := make([]itemRecord, 0, len(kept))
				for _, item := range kept {
					items = append(items, itemRecord{ID: uuid.New(), ComparisonListID: userListID, ProductVariantID: item.ProductVariantID, CreatedAt: item.CreatedAt})
				}
				if err := tx.Create(&items).Error; err != nil {
					return err
				}
			}
		}
		return tx.Where("session_id = ?", sessionID).Delete(&listRecord{}).Error
	})
}

func newestUniqueItems(items []itemRecord, maxItems int) []itemRecord {
	newestByVariant := make(map[uuid.UUID]itemRecord, len(items))
	for _, item := range items {
		current, exists := newestByVariant[item.ProductVariantID]
		if !exists || item.CreatedAt.After(current.CreatedAt) || (item.CreatedAt.Equal(current.CreatedAt) && item.ID.String() > current.ID.String()) {
			newestByVariant[item.ProductVariantID] = item
		}
	}
	result := make([]itemRecord, 0, len(newestByVariant))
	for _, item := range newestByVariant {
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].CreatedAt.Equal(result[right].CreatedAt) {
			return result[left].ID.String() > result[right].ID.String()
		}
		return result[left].CreatedAt.After(result[right].CreatedAt)
	})
	if len(result) > maxItems {
		return result[:maxItems]
	}
	return result
}

func productCategory(ctx context.Context, db *gorm.DB, variantID uuid.UUID) (uuid.UUID, error) {
	var categoryID uuid.UUID
	err := db.WithContext(ctx).Raw(`
		SELECT products.category_id
		FROM product_variants
		JOIN products ON products.id = product_variants.product_id
		WHERE product_variants.id = ?
		  AND product_variants.status = 'active'
		  AND products.status = 'active'
		  AND products.category_id IS NOT NULL`, variantID).Scan(&categoryID).Error
	if err != nil {
		return uuid.Nil, err
	}
	if categoryID == uuid.Nil {
		return uuid.Nil, domain.ErrVariantNotFound
	}
	return categoryID, nil
}

func ensureList(ctx context.Context, db *gorm.DB, owner domain.Owner, categoryID uuid.UUID) (uuid.UUID, error) {
	var listID uuid.UUID
	var err error
	if owner.UserID != nil {
		err = db.WithContext(ctx).Raw(`
			INSERT INTO comparison_lists (id, user_id, category_id)
			VALUES (?, ?, ?)
			ON CONFLICT (user_id, category_id) WHERE user_id IS NOT NULL
			DO UPDATE SET id = comparison_lists.id
			RETURNING id`, uuid.New(), *owner.UserID, categoryID).Scan(&listID).Error
	} else if owner.SessionID != nil {
		err = db.WithContext(ctx).Raw(`
			INSERT INTO comparison_lists (id, session_id, category_id)
			VALUES (?, ?, ?)
			ON CONFLICT (session_id, category_id) WHERE session_id IS NOT NULL
			DO UPDATE SET id = comparison_lists.id
			RETURNING id`, uuid.New(), *owner.SessionID, categoryID).Scan(&listID).Error
	} else {
		return uuid.Nil, domain.ErrInvalidOwner
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("ensure comparison list: %w", err)
	}
	if listID == uuid.Nil {
		return uuid.Nil, errors.New("ensure comparison list returned no id")
	}
	return listID, nil
}

type listRecord struct {
	ID         uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID     *uuid.UUID `gorm:"column:user_id;type:uuid"`
	SessionID  *uuid.UUID `gorm:"column:session_id;type:uuid"`
	CategoryID uuid.UUID  `gorm:"column:category_id;type:uuid"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
}

func (listRecord) TableName() string { return "comparison_lists" }

type itemRecord struct {
	ID               uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	ComparisonListID uuid.UUID `gorm:"column:comparison_list_id;type:uuid"`
	ProductVariantID uuid.UUID `gorm:"column:product_variant_id;type:uuid"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (itemRecord) TableName() string { return "comparison_items" }

var _ domain.Repository = (*Repository)(nil)

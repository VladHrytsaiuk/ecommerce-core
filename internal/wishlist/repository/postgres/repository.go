// Package postgres implements Wishlist persistence. SQL is used for the
// partial-index conflict targets needed for owner-specific idempotency.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, owner domain.Owner) ([]domain.WishlistItem, error) {
	if !owner.Valid() {
		return nil, domain.ErrInvalidOwner
	}
	var records []record
	query := r.db.WithContext(ctx).Order("created_at DESC")
	if owner.UserID != nil {
		query = query.Where("user_id = ?", *owner.UserID)
	} else {
		query = query.Where("session_id = ?", *owner.SessionID)
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	items := make([]domain.WishlistItem, 0, len(records))
	for _, item := range records {
		items = append(items, domain.WishlistItem{ID: item.ID, ProductVariantID: item.ProductVariantID, CreatedAt: item.CreatedAt})
	}
	return items, nil
}

func (r *Repository) Add(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	if owner.UserID != nil {
		return r.db.WithContext(ctx).Exec(`
			INSERT INTO wishlist_items (id, user_id, product_variant_id)
			VALUES (?, ?, ?)
			ON CONFLICT (user_id, product_variant_id) WHERE user_id IS NOT NULL DO NOTHING`, uuid.New(), *owner.UserID, variantID).Error
	}
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO wishlist_items (id, session_id, product_variant_id)
		VALUES (?, ?, ?)
		ON CONFLICT (session_id, product_variant_id) WHERE session_id IS NOT NULL DO NOTHING`, uuid.New(), *owner.SessionID, variantID).Error
}

func (r *Repository) Remove(ctx context.Context, owner domain.Owner, variantID uuid.UUID) error {
	if !owner.Valid() {
		return domain.ErrInvalidOwner
	}
	query := r.db.WithContext(ctx).Where("product_variant_id = ?", variantID)
	if owner.UserID != nil {
		query = query.Where("user_id = ?", *owner.UserID)
	} else {
		query = query.Where("session_id = ?", *owner.SessionID)
	}
	return query.Delete(&record{}).Error
}

// MergeGuestWishlist is one local transaction: it copies non-duplicate guest
// variants, then removes every source item only after the insert succeeds.
func (r *Repository) MergeGuestWishlist(ctx context.Context, userID, sessionID uuid.UUID) error {
	if userID == uuid.Nil || sessionID == uuid.Nil {
		return domain.ErrInvalidOwner
	}
	return transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO wishlist_items (id, user_id, product_variant_id, created_at)
			SELECT gen_random_uuid(), ?, product_variant_id, created_at
			FROM wishlist_items
			WHERE session_id = ?
			ON CONFLICT (user_id, product_variant_id) WHERE user_id IS NOT NULL DO NOTHING`, userID, sessionID).Error; err != nil {
			return fmt.Errorf("copy guest wishlist items: %w", err)
		}
		if err := tx.Where("session_id = ?", sessionID).Delete(&record{}).Error; err != nil {
			return fmt.Errorf("delete guest wishlist items: %w", err)
		}
		return nil
	})
}

type record struct {
	ID               uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	UserID           *uuid.UUID `gorm:"column:user_id;type:uuid"`
	SessionID        *uuid.UUID `gorm:"column:session_id;type:uuid"`
	ProductVariantID uuid.UUID  `gorm:"column:product_variant_id;type:uuid"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (record) TableName() string { return "wishlist_items" }

var _ domain.Repository = (*Repository)(nil)

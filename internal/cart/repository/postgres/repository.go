package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type cartRecord struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"`
	CustomerID, SessionID *uuid.UUID
	Status                string
	AppliedPromoCode      *string
	DiscountAmount        int64
	UpdatedAt             time.Time
}

func (cartRecord) TableName() string { return "carts" }

type itemRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	CartID, VariantID uuid.UUID
	Quantity          int
}

func (itemRecord) TableName() string { return "cart_items" }

func (r *Repository) GetOrCreate(ctx context.Context, owner domain.Owner) (*domain.Cart, error) {
	var result *domain.Cart
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		cart, err := findOrCreate(tx, owner)
		if err != nil {
			return err
		}
		result, err = load(tx, cart, owner)
		return err
	})
	return result, err
}
func (r *Repository) Add(ctx context.Context, owner domain.Owner, item domain.Item) (*domain.Cart, error) {
	return r.mutate(ctx, owner, func(tx *gorm.DB, cart *cartRecord) error {
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "cart_id"}, {Name: "variant_id"}}, DoUpdates: clause.Assignments(map[string]any{"quantity": gorm.Expr("cart_items.quantity + ?", item.Quantity)})}).Create(&itemRecord{ID: uuid.New(), CartID: cart.ID, VariantID: item.VariantID, Quantity: item.Quantity}).Error
	})
}
func (r *Repository) SetQuantity(ctx context.Context, owner domain.Owner, item domain.Item) (*domain.Cart, error) {
	return r.mutate(ctx, owner, func(tx *gorm.DB, cart *cartRecord) error {
		result := tx.Model(&itemRecord{}).Where("cart_id = ? AND variant_id = ?", cart.ID, item.VariantID).Update("quantity", item.Quantity)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrItemNotFound
		}
		return nil
	})
}
func (r *Repository) Remove(ctx context.Context, owner domain.Owner, variantID uuid.UUID) (*domain.Cart, error) {
	return r.mutate(ctx, owner, func(tx *gorm.DB, cart *cartRecord) error {
		result := tx.Where("cart_id = ? AND variant_id = ?", cart.ID, variantID).Delete(&itemRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrItemNotFound
		}
		return nil
	})
}
func (r *Repository) SetPromoCode(ctx context.Context, owner domain.Owner, code string) (*domain.Cart, error) {
	return r.mutate(ctx, owner, func(tx *gorm.DB, cart *cartRecord) error {
		values := map[string]any{"applied_promo_code": nil, "discount_amount": int64(0)}
		if code != "" {
			values["applied_promo_code"] = code
		}
		return tx.Model(cart).Updates(values).Error
	})
}
func (r *Repository) mutate(ctx context.Context, owner domain.Owner, change func(*gorm.DB, *cartRecord) error) (*domain.Cart, error) {
	var result *domain.Cart
	err := transaction.Within(ctx, r.db, func(tx *gorm.DB) error {
		cart, err := findOrCreate(tx, owner)
		if err != nil {
			return err
		}
		if err := change(tx, cart); err != nil {
			return err
		}
		if err := tx.Model(cart).Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
			return err
		}
		result, err = load(tx, cart, owner)
		return err
	})
	return result, err
}
func findOrCreate(tx *gorm.DB, owner domain.Owner) (*cartRecord, error) {
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status = ?", "active")
	if owner.CustomerID != nil {
		query = query.Where("customer_id = ?", *owner.CustomerID)
	} else {
		query = query.Where("session_id = ?", *owner.SessionID)
	}
	var cart cartRecord
	err := query.First(&cart).Error
	if err == nil {
		return &cart, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	cart = cartRecord{ID: uuid.New(), CustomerID: owner.CustomerID, SessionID: owner.SessionID, Status: "active"}
	if err := tx.Create(&cart).Error; err != nil {
		return nil, err
	}
	return &cart, nil
}
func load(tx *gorm.DB, record *cartRecord, owner domain.Owner) (*domain.Cart, error) {
	var rows []itemRecord
	if err := tx.Where("cart_id = ?", record.ID).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]domain.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.Item{VariantID: row.VariantID, Quantity: row.Quantity})
	}
	code := ""
	if record.AppliedPromoCode != nil {
		code = *record.AppliedPromoCode
	}
	return &domain.Cart{ID: record.ID, Owner: owner, Status: record.Status, Items: items, AppliedPromoCode: code, DiscountAmount: record.DiscountAmount, UpdatedAt: record.UpdatedAt}, nil
}

var _ domain.Repository = (*Repository)(nil)

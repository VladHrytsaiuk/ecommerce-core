package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	ordersDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/orders/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	promosDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/promos/domain"
)

type Repository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db, now: func() time.Time { return time.Now().UTC() }}
}

type codeRecord struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Code          string
	DiscountType  string
	DiscountValue int64
	Currency      *string
	IsActive      bool
	ValidUntil    *time.Time
	UsageLimit    *int
	UsageCount    int
}

func (codeRecord) TableName() string { return "promocodes" }

type redemptionRecord struct {
	PromoID uuid.UUID
	OrderID uuid.UUID `gorm:"primaryKey"`
	Status  string
}

func (redemptionRecord) TableName() string { return "promo_redemptions" }

func (r *Repository) FindByCode(ctx context.Context, code string) (*promosDomain.Code, error) {
	var record codeRecord
	if err := r.db.WithContext(ctx).Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, promosDomain.ErrCodeNotFound
		}
		return nil, err
	}
	return mapCode(record), nil
}

func (r *Repository) Reserve(ctx context.Context, orderID uuid.UUID, snapshot ordersDomain.Promotion) error {
	tx, err := transaction.FromContext(ctx)
	if err != nil {
		return err
	}
	var existing redemptionRecord
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "order_id = ?", orderID).Error
	if err == nil {
		if existing.Status == "reserved" || existing.Status == "committed" {
			return nil
		}
		return promosDomain.ErrUsageExhausted
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var promo codeRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&promo, "code = ?", snapshot.Code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return promosDomain.ErrCodeNotFound
		}
		return err
	}
	if !promo.IsActive {
		return promosDomain.ErrCodeInactive
	}
	if promo.ValidUntil != nil && !promo.ValidUntil.After(r.now()) {
		return promosDomain.ErrCodeExpired
	}
	if promo.DiscountType != snapshot.Type || promo.DiscountValue != snapshot.Value || valueOrEmpty(promo.Currency) != snapshot.Currency {
		return promosDomain.ErrInvalidCode
	}
	if promo.UsageLimit != nil {
		var reserved int64
		if err := tx.Model(&redemptionRecord{}).Where("promo_id = ? AND status = ?", promo.ID, "reserved").Count(&reserved).Error; err != nil {
			return err
		}
		if int64(promo.UsageCount)+reserved >= int64(*promo.UsageLimit) {
			return promosDomain.ErrUsageExhausted
		}
	}
	return tx.Create(&redemptionRecord{PromoID: promo.ID, OrderID: orderID, Status: "reserved"}).Error
}

func (r *Repository) Commit(ctx context.Context, orderID uuid.UUID) error {
	tx, err := transaction.FromContext(ctx)
	if err != nil {
		return err
	}
	var redemption redemptionRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&redemption, "order_id = ?", orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if redemption.Status == "committed" || redemption.Status == "released" {
		return nil
	}
	if redemption.Status != "reserved" {
		return promosDomain.ErrInvalidCode
	}
	var promo codeRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&promo, "id = ?", redemption.PromoID).Error; err != nil {
		return err
	}
	if promo.UsageLimit != nil && promo.UsageCount >= *promo.UsageLimit {
		return promosDomain.ErrUsageExhausted
	}
	if err := tx.Model(&redemption).Updates(map[string]any{"status": "committed", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		return err
	}
	return tx.Model(&promo).Updates(map[string]any{"usage_count": gorm.Expr("usage_count + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}

func (r *Repository) Release(ctx context.Context, orderID uuid.UUID) error {
	tx, err := transaction.FromContext(ctx)
	if err != nil {
		return err
	}
	return tx.Model(&redemptionRecord{}).Where("order_id = ? AND status = ?", orderID, "reserved").Updates(map[string]any{"status": "released", "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
}

func mapCode(record codeRecord) *promosDomain.Code {
	return &promosDomain.Code{ID: record.ID, Code: record.Code, DiscountType: record.DiscountType, DiscountValue: record.DiscountValue, Currency: valueOrEmpty(record.Currency), IsActive: record.IsActive, ValidUntil: record.ValidUntil, UsageLimit: record.UsageLimit, UsageCount: record.UsageCount}
}
func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ promosDomain.Repository = (*Repository)(nil)

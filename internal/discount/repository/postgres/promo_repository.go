package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"gorm.io/gorm"
)

type promoRepository struct {
	db *gorm.DB
	l  logger.Logger
}

func NewPromoRepository(db *gorm.DB, l logger.Logger) domain.PromoRepository {
	return &promoRepository{db: db, l: l}
}

func (r *promoRepository) loadRelations(ctx context.Context, p *domain.PromoCode) error {
	var categoryIDs []uuid.UUID
	if err := r.db.WithContext(ctx).Table("promo_code_category").Where("promo_code_id = ?", p.ID).Pluck("category_id", &categoryIDs).Error; err != nil {
		return err
	}
	p.CategoryIDs = categoryIDs

	var brandIDs []uuid.UUID
	if err := r.db.WithContext(ctx).Table("promo_code_brand").Where("promo_code_id = ?", p.ID).Pluck("brand_id", &brandIDs).Error; err != nil {
		return err
	}
	p.BrandIDs = brandIDs

	var productIDs []uuid.UUID
	if err := r.db.WithContext(ctx).Table("promo_code_product").Where("promo_code_id = ?", p.ID).Pluck("product_id", &productIDs).Error; err != nil {
		return err
	}
	p.ProductIDs = productIDs

	return nil
}

func (r *promoRepository) saveRelations(ctx context.Context, tx *gorm.DB, p *domain.PromoCode) error {
	if err := tx.WithContext(ctx).Exec("DELETE FROM promo_code_category WHERE promo_code_id = ?", p.ID).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec("DELETE FROM promo_code_brand WHERE promo_code_id = ?", p.ID).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec("DELETE FROM promo_code_product WHERE promo_code_id = ?", p.ID).Error; err != nil {
		return err
	}

	for _, cid := range p.CategoryIDs {
		if err := tx.WithContext(ctx).Exec("INSERT INTO promo_code_category (promo_code_id, category_id) VALUES (?, ?)", p.ID, cid).Error; err != nil {
			return err
		}
	}
	for _, bid := range p.BrandIDs {
		if err := tx.WithContext(ctx).Exec("INSERT INTO promo_code_brand (promo_code_id, brand_id) VALUES (?, ?)", p.ID, bid).Error; err != nil {
			return err
		}
	}
	for _, pid := range p.ProductIDs {
		if err := tx.WithContext(ctx).Exec("INSERT INTO promo_code_product (promo_code_id, product_id) VALUES (?, ?)", p.ID, pid).Error; err != nil {
			return err
		}
	}

	return nil
}

func (r *promoRepository) Create(ctx context.Context, p *domain.PromoCode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		return r.saveRelations(ctx, tx, p)
	})
}

func (r *promoRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.PromoCode, error) {
	var p domain.PromoCode
	if err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPromoCodeNotFound
		}
		return nil, err
	}
	if err := r.loadRelations(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *promoRepository) GetByCode(ctx context.Context, code string) (*domain.PromoCode, error) {
	var p domain.PromoCode
	// Промокоди регістронезалежні: в адмінці код зберігається у верхньому регістрі,
	// але покупець може ввести його будь-яким регістром.
	if err := r.db.WithContext(ctx).First(&p, "UPPER(code) = ?", strings.ToUpper(code)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPromoCodeNotFound
		}
		return nil, err
	}
	if err := r.loadRelations(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *promoRepository) Update(ctx context.Context, p *domain.PromoCode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(p).Error; err != nil {
			return err
		}
		return r.saveRelations(ctx, tx, p)
	})
}

func (r *promoRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&domain.PromoCode{}, "id = ?", id).Error
}

func (r *promoRepository) List(ctx context.Context, page, limit int) ([]*domain.PromoCode, int64, error) {
	var promos []*domain.PromoCode
	var total int64

	offset := (page - 1) * limit

	if err := r.db.WithContext(ctx).Model(&domain.PromoCode{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).Order("created_at desc").Offset(offset).Limit(limit).Find(&promos).Error; err != nil {
		return nil, 0, err
	}

	for _, p := range promos {
		if err := r.loadRelations(ctx, p); err != nil {
			return nil, 0, err
		}
	}

	return promos, total, nil
}

func (r *promoRepository) RecordUsage(ctx context.Context, usage *domain.PromoCodeUsage) error {
	return r.db.WithContext(ctx).Create(usage).Error
}

func (r *promoRepository) CheckUserUsage(ctx context.Context, promoID uuid.UUID, userID *uuid.UUID, email, phone *string) (int, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&domain.PromoCodeUsage{}).Where("promo_code_id = ?", promoID)

	if userID != nil {
		query = query.Where("user_id = ?", userID)
	} else {
		var conds []string
		var args []interface{}
		if email != nil && *email != "" {
			conds = append(conds, "email = ?")
			args = append(args, *email)
		}
		if phone != nil && *phone != "" {
			conds = append(conds, "phone = ?")
			args = append(args, *phone)
		}
		
		if len(conds) == 0 {
			return 0, nil
		}
		
		query = query.Where(joinConds(conds, " OR "), args...)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}

	return int(count), nil
}

func joinConds(conds []string, sep string) string {
	if len(conds) == 0 {
		return ""
	}
	res := conds[0]
	for i := 1; i < len(conds); i++ {
		res += sep + conds[i]
	}
	return res
}

func (r *promoRepository) IncrementUsageCount(ctx context.Context, promoID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&domain.PromoCode{}).Where("id = ?", promoID).UpdateColumn("usage_count", gorm.Expr("usage_count + ?", 1)).Error
}

func (r *promoRepository) IncrementUsageAtomic(ctx context.Context, promoID uuid.UUID) error {
	tx := r.db.WithContext(ctx).Exec(`
		UPDATE promo_code 
		SET usage_count = usage_count + 1 
		WHERE id = ? 
		  AND is_active = true 
		  AND (ends_at IS NULL OR ends_at > NOW()) 
		  AND (usage_limit IS NULL OR usage_count < usage_limit)
	`, promoID)
	
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected == 0 {
		return domain.ErrPromoCodeLimitExceeded
	}
	return nil
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"gorm.io/gorm"
)

type attributeRepository struct {
	db *gorm.DB
	l  logger.Logger
}

func NewAttributeRepository(db *gorm.DB, l logger.Logger) domain.AttributeRepository {
	return &attributeRepository{db: db, l: l}
}

func (r *attributeRepository) FindAll(ctx context.Context, lang string) ([]domain.Attribute, error) {
	var attributes []domain.Attribute
	err := r.db.WithContext(ctx).
		Preload("Translations", "language_code = ?", lang).
		Preload("Unit").
		Order("sort_order ASC").
		Find(&attributes).Error
	if err != nil {
		r.l.Errorw("failed to find attributes", "error", err)
		return nil, err
	}
	return attributes, nil
}

func (r *attributeRepository) FindByID(ctx context.Context, id int, lang string) (*domain.Attribute, error) {
	var attribute domain.Attribute
	err := r.db.WithContext(ctx).
		Preload("Translations", "language_code = ?", lang).
		Preload("Unit").
		Where("id = ?", id).
		First(&attribute).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("attribute not found")
		}
		r.l.Errorw("failed to find attribute by id", "error", err, "id", id)
		return nil, err
	}
	return &attribute, nil
}

// FindValues повертає всі унікальні значення характеристики за її кодом (напр. "type"),
// незалежно від активності товарів. На відміну від GetFilters (вітрина, лише активні
// фільтровані значення), це джерело для підказок в адмінці — тому тип, використаний
// навіть у чернетці/неактивному товарі, лишається доступним для вибору.
func (r *attributeRepository) FindValues(ctx context.Context, attributeCode, lang string) ([]domain.AttrValueOption, error) {
	// Валідація мови (labelExpr інтерполюється в SQL) — запобігання ін'єкції.
	if lang != "uk" && lang != "en" {
		lang = "uk"
	}
	labelExpr := fmt.Sprintf("COALESCE(av.value_string->>'%s', av.value_string->>'uk', CAST(av.value_numeric AS TEXT))", lang)

	var values []domain.AttrValueOption
	err := r.db.WithContext(ctx).
		Table("attribute_value av").
		Joins("JOIN attribute a ON a.id = av.attribute_id AND a.deleted_at IS NULL").
		Where("a.code = ?", attributeCode).
		Select("av.value_code as code, "+labelExpr+" as label, COUNT(*) as count").
		Group("av.value_code, " + labelExpr).
		Order("label ASC").
		Scan(&values).Error
	if err != nil {
		r.l.Errorw("failed to find attribute values", "error", err, "attr_code", attributeCode)
		return nil, err
	}

	// Відкидаємо порожні мітки (напр. запис лише з value_numeric без строкового значення).
	out := make([]domain.AttrValueOption, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v.Label) == "" {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *attributeRepository) Create(ctx context.Context, attribute *domain.Attribute) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(attribute).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *attributeRepository) Update(ctx context.Context, attribute *domain.Attribute) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(attribute).Error; err != nil {
			return err
		}

		if len(attribute.Translations) > 0 {
			if err := tx.Where("attribute_id = ?", attribute.ID).Delete(&domain.AttributeTranslation{}).Error; err != nil {
				return err
			}
			if err := tx.Create(&attribute.Translations).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *attributeRepository) Delete(ctx context.Context, id int) error {
	return r.db.WithContext(ctx).Delete(&domain.Attribute{}, id).Error
}

func (r *attributeRepository) UpdateOrder(ctx context.Context, ids []int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&domain.Attribute{}).Where("id = ?", id).UpdateColumn("sort_order", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

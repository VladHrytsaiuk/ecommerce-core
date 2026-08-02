package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
)

type CategoryRepository struct{ db *gorm.DB }

func NewCategoryRepository(db *gorm.DB) *CategoryRepository { return &CategoryRepository{db: db} }

func (r *CategoryRepository) FindBySlug(ctx context.Context, locale, slug string) (*domain.Category, error) {
	var category domain.Category
	err := r.db.WithContext(ctx).
		Joins("JOIN category_translations ct ON ct.category_id = categories.id").
		Preload("Translations").
		Where("ct.locale = ? AND ct.slug = ?", locale, slug).
		First(&category).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCatalogCategoryNotFound
		}
		return nil, err
	}
	return &category, nil
}

func (r *CategoryRepository) Create(ctx context.Context, category *domain.Category) error {
	if category.ID == uuid.Nil {
		category.ID = uuid.New()
	}
	for i := range category.Translations {
		category.Translations[i].CategoryID = category.ID
	}
	return r.db.WithContext(ctx).Create(category).Error
}

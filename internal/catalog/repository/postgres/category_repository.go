package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
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

func (r *CategoryRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
	var category domain.Category
	if err := r.database(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Translations").First(&category, "id = ?", id).Error; err != nil {
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
	return r.database(ctx).Create(category).Error
}

func (r *CategoryRepository) Update(ctx context.Context, category *domain.Category) error {
	for i := range category.Translations {
		category.Translations[i].CategoryID = category.ID
	}
	db := r.database(ctx)
	if err := db.Model(&domain.Category{}).Where("id = ?", category.ID).Updates(map[string]any{"parent_id": category.ParentID, "sort_order": category.SortOrder, "is_active": category.IsActive, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
		return err
	}
	if err := db.Where("category_id = ?", category.ID).Delete(&domain.CategoryTranslation{}).Error; err != nil {
		return err
	}
	return db.Create(&category.Translations).Error
}

func (r *CategoryRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

var _ domain.AdminCategoryRepository = (*CategoryRepository)(nil)

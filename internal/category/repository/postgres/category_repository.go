package postgres

import (
	"context"
	"errors"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type categoryRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewCategoryRepository створює новий інстанс репозиторію
func NewCategoryRepository(db *gorm.DB, l logger.Logger) domain.CategoryRepository {
	return &categoryRepository{db: db, l: l}
}

// FindAll завантажує всі категорії включно з заданим перекладом
func (r *categoryRepository) FindAll(ctx context.Context, lang string) ([]domain.Category, error) {
	var categories []domain.Category

	err := r.db.WithContext(ctx).
		Preload("Translations").
		Order("sort_order ASC, id ASC").
		Find(&categories).Error

	if err != nil {
		r.l.Errorw("failed to find all categories", "error", err)
		return nil, err
	}

	return categories, nil
}

// FindByID завантажує інформацію про конкретну категорію
func (r *categoryRepository) FindByID(ctx context.Context, id uuid.UUID, lang string) (*domain.Category, error) {
	var category domain.Category

	var err error
	if lang == "all" || lang == "" {
		err = r.db.WithContext(ctx).
			Preload("Translations").
			Where("id = ?", id).
			First(&category).Error
	} else {
		err = r.db.WithContext(ctx).
			Preload("Translations").
			Where("id = ?", id).
			First(&category).Error
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCategoryNotFound
		}
		r.l.Errorw("failed to find category by id", "error", err, "id", id)
		return nil, err
	}

	return &category, nil
}

func (r *categoryRepository) FindBySlug(ctx context.Context, slug string, lang string) (*domain.Category, error) {
	var category domain.Category

	err := r.db.WithContext(ctx).
		Joins("JOIN category_translations ct ON ct.category_id = categories.id").
		Preload("Translations").
		Where("ct.slug = ? AND ct.locale = ?", slug, lang).
		First(&category).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCategoryNotFound
		}
		r.l.Errorw("failed to find category by slug", "slug", slug, "error", err)
		return nil, err
	}

	return &category, nil
}

func (r *categoryRepository) Create(ctx context.Context, category *domain.Category) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Open gap for the new category's sort_order (largest sort_order first to avoid conflicts)
		var siblings []domain.Category
		var query *gorm.DB
		if category.ParentID == nil {
			query = tx.Where("parent_id IS NULL AND sort_order >= ?", category.SortOrder)
		} else {
			query = tx.Where("parent_id = ? AND sort_order >= ?", category.ParentID, category.SortOrder)
		}
		if err := query.Order("sort_order DESC").Find(&siblings).Error; err != nil {
			r.l.Errorw("failed to fetch sibling categories for creation shift", "error", err)
			return err
		}
		for _, sib := range siblings {
			if err := tx.Model(&domain.Category{}).Where("id = ?", sib.ID).UpdateColumn("sort_order", sib.SortOrder+1).Error; err != nil {
				r.l.Errorw("failed to shift sibling category up", "error", err, "id", sib.ID)
				return err
			}
		}

		if err := tx.Create(category).Error; err != nil {
			r.l.Errorw("failed to create category", "error", err)
			return err
		}
		return nil
	})
}

func (r *categoryRepository) Update(ctx context.Context, category *domain.Category) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Отримуємо існуючу категорію з БД, щоб перевірити зміни в ParentID або SortOrder
		var existing domain.Category
		if err := tx.Where("id = ?", category.ID).First(&existing).Error; err != nil {
			r.l.Errorw("failed to fetch existing category for update check", "error", err, "id", category.ID)
			return err
		}

		sortOrderChanged := existing.SortOrder != category.SortOrder
		parentIDChanged := (existing.ParentID == nil && category.ParentID != nil) ||
			(existing.ParentID != nil && category.ParentID == nil) ||
			(existing.ParentID != nil && category.ParentID != nil && *existing.ParentID != *category.ParentID)

		if parentIDChanged || sortOrderChanged {
			// Тимчасово прибираємо поточну категорію в індекс -1 для уникнення конфліктів унікальності
			if err := tx.Model(&domain.Category{}).Where("id = ?", category.ID).UpdateColumn("sort_order", -1).Error; err != nil {
				r.l.Errorw("failed to move category to temporary sort order", "error", err, "id", category.ID)
				return err
			}
		}

		if parentIDChanged {
			// Закриваємо пробіл у старій батьківській гілці (найменший sort_order першим для зсуву вниз)
			var siblingsOld []domain.Category
			var oldParentQuery *gorm.DB
			if existing.ParentID == nil {
				oldParentQuery = tx.Where("parent_id IS NULL AND sort_order > ? AND id != ?", existing.SortOrder, category.ID)
			} else {
				oldParentQuery = tx.Where("parent_id = ? AND sort_order > ? AND id != ?", existing.ParentID, existing.SortOrder, category.ID)
			}
			if err := oldParentQuery.Order("sort_order ASC").Find(&siblingsOld).Error; err != nil {
				r.l.Errorw("failed to fetch old parent siblings", "error", err)
				return err
			}
			for _, sib := range siblingsOld {
				if err := tx.Model(&domain.Category{}).Where("id = ?", sib.ID).UpdateColumn("sort_order", sib.SortOrder-1).Error; err != nil {
					r.l.Errorw("failed to shift old sibling category down", "error", err, "id", sib.ID)
					return err
				}
			}

			// Відкриваємо пробіл у новій батьківській гілці (найбільший sort_order першим для зсуву вгору)
			var siblingsNew []domain.Category
			var newParentQuery *gorm.DB
			if category.ParentID == nil {
				newParentQuery = tx.Where("parent_id IS NULL AND sort_order >= ? AND id != ?", category.SortOrder, category.ID)
			} else {
				newParentQuery = tx.Where("parent_id = ? AND sort_order >= ? AND id != ?", category.ParentID, category.SortOrder, category.ID)
			}
			if err := newParentQuery.Order("sort_order DESC").Find(&siblingsNew).Error; err != nil {
				r.l.Errorw("failed to fetch new parent siblings", "error", err)
				return err
			}
			for _, sib := range siblingsNew {
				if err := tx.Model(&domain.Category{}).Where("id = ?", sib.ID).UpdateColumn("sort_order", sib.SortOrder+1).Error; err != nil {
					r.l.Errorw("failed to shift new sibling category up", "error", err, "id", sib.ID)
					return err
				}
			}
		} else if sortOrderChanged {
			var siblings []domain.Category
			var query *gorm.DB
			if category.SortOrder < existing.SortOrder {
				// Зсуваємо елементи в [new_order, old_order - 1] на +1 (найбільший sort_order першим)
				if category.ParentID == nil {
					query = tx.Where("parent_id IS NULL AND sort_order >= ? AND sort_order < ? AND id != ?", category.SortOrder, existing.SortOrder, category.ID)
				} else {
					query = tx.Where("parent_id = ? AND sort_order >= ? AND sort_order < ? AND id != ?", category.ParentID, category.SortOrder, existing.SortOrder, category.ID)
				}
				if err := query.Order("sort_order DESC").Find(&siblings).Error; err != nil {
					r.l.Errorw("failed to fetch siblings for shifting up", "error", err)
					return err
				}
				for _, sib := range siblings {
					if err := tx.Model(&domain.Category{}).Where("id = ?", sib.ID).UpdateColumn("sort_order", sib.SortOrder+1).Error; err != nil {
						r.l.Errorw("failed to shift sibling up", "error", err, "id", sib.ID)
						return err
					}
				}
			} else {
				// Зсуваємо елементи в [old_order + 1, new_order] на -1 (найменший sort_order першим)
				if category.ParentID == nil {
					query = tx.Where("parent_id IS NULL AND sort_order > ? AND sort_order <= ? AND id != ?", existing.SortOrder, category.SortOrder, category.ID)
				} else {
					query = tx.Where("parent_id = ? AND sort_order > ? AND sort_order <= ? AND id != ?", category.ParentID, existing.SortOrder, category.SortOrder, category.ID)
				}
				if err := query.Order("sort_order ASC").Find(&siblings).Error; err != nil {
					r.l.Errorw("failed to fetch siblings for shifting down", "error", err)
					return err
				}
				for _, sib := range siblings {
					if err := tx.Model(&domain.Category{}).Where("id = ?", sib.ID).UpdateColumn("sort_order", sib.SortOrder-1).Error; err != nil {
						r.l.Errorw("failed to shift sibling down", "error", err, "id", sib.ID)
						return err
					}
				}
			}
		}

		// 2. Оновлюємо саму категорію
		if err := tx.Save(category).Error; err != nil {
			r.l.Errorw("failed to save category in update", "error", err, "id", category.ID)
			return err
		}

		// 3. Якщо переклади передані — синхронізуємо їх (повністю замінюємо для цього запиту)
		if len(category.Translations) > 0 {
			if err := tx.Where("category_id = ?", category.ID).Delete(&domain.CategoryTranslation{}).Error; err != nil {
				return err
			}
			if err := tx.Create(&category.Translations).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *categoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).Delete(&domain.Category{}, id).Error
	if err != nil {
		r.l.Errorw("failed to delete category", "error", err, "id", id)
		return err
	}
	return nil
}

func (r *categoryRepository) UpdateOrder(ctx context.Context, ids []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Тимчасово зсуваємо всі категорії у від'ємний діапазон, щоб уникнути конфліктів унікальності
		for i, id := range ids {
			tempSortOrder := -1 - i
			if err := tx.Model(&domain.Category{}).Where("id = ?", id).UpdateColumn("sort_order", tempSortOrder).Error; err != nil {
				r.l.Errorw("failed to temporarily shift category order on bulk update", "error", err, "id", id)
				return err
			}
		}

		// 2. Встановлюємо фінальні порядкові номери
		for i, id := range ids {
			if err := tx.Model(&domain.Category{}).Where("id = ?", id).UpdateColumn("sort_order", i).Error; err != nil {
				r.l.Errorw("failed to set final category order on bulk update", "error", err, "id", id)
				return err
			}
		}
		return nil
	})
}

func (r *categoryRepository) SlugExists(ctx context.Context, slug string, excludeID uuid.UUID) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&domain.CategoryTranslation{}).Where("slug = ?", slug)
	if excludeID != uuid.Nil {
		query = query.Where("category_id != ?", excludeID)
	}
	err := query.Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check category slug existence", "error", err, "slug", slug)
		return false, err
	}
	return count > 0, nil
}

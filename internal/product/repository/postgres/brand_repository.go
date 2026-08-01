package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"gorm.io/gorm"
)

type brandRepository struct {
	db *gorm.DB
	l  logger.Logger
}

func NewBrandRepository(db *gorm.DB, l logger.Logger) domain.BrandRepository {
	return &brandRepository{db: db, l: l}
}

func (r *brandRepository) FindAll(ctx context.Context) ([]domain.Brand, error) {
	var brands []domain.Brand
	err := r.db.WithContext(ctx).Order("name ASC").Find(&brands).Error
	if err != nil {
		r.l.Errorw("failed to find brands", "error", err)
		return nil, err
	}
	return brands, nil
}

func (r *brandRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Brand, error) {
	var brand domain.Brand
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&brand).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrBrandNotFound
		}
		r.l.Errorw("failed to find brand by id", "error", err, "id", id)
		return nil, err
	}
	return &brand, nil
}

func (r *brandRepository) Create(ctx context.Context, brand *domain.Brand) error {
	err := r.db.WithContext(ctx).Create(brand).Error
	if err != nil {
		r.l.Errorw("failed to create brand", "error", err)
		return err
	}
	return nil
}

func (r *brandRepository) Update(ctx context.Context, brand *domain.Brand) error {
	err := r.db.WithContext(ctx).Save(brand).Error
	if err != nil {
		r.l.Errorw("failed to update brand", "error", err, "id", brand.ID)
		return err
	}
	return nil
}

func (r *brandRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Brand{}).Error
	if err != nil {
		r.l.Errorw("failed to delete brand", "error", err, "id", id)
		return err
	}
	return nil
}

func (r *brandRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Brand{}).
		Where("slug = ?", slug).Count(&count).Error
	if err != nil {
		r.l.Errorw("failed to check brand slug existence", "error", err, "slug", slug)
		return false, err
	}
	return count > 0, nil
}

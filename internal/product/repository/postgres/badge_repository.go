package postgres

import (
	"context"
	"errors"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"gorm.io/gorm"
)

type badgeRepository struct {
	db *gorm.DB
	l  logger.Logger
}

func NewBadgeRepository(db *gorm.DB, l logger.Logger) domain.BadgeRepository {
	return &badgeRepository{db: db, l: l}
}

func (r *badgeRepository) FindAll(ctx context.Context) ([]domain.Badge, error) {
	var badges []domain.Badge
	err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&badges).Error
	if err != nil {
		r.l.Errorw("failed to find badges", "error", err)
		return nil, err
	}
	return badges, nil
}

func (r *badgeRepository) FindByID(ctx context.Context, id int) (*domain.Badge, error) {
	var badge domain.Badge
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&badge).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrBadgeNotFound
		}
		r.l.Errorw("failed to find badge by id", "error", err, "id", id)
		return nil, err
	}
	return &badge, nil
}

func (r *badgeRepository) Create(ctx context.Context, badge *domain.Badge) error {
	err := r.db.WithContext(ctx).Create(badge).Error
	if err != nil {
		r.l.Errorw("failed to create badge", "error", err)
		return err
	}
	return nil
}

func (r *badgeRepository) Update(ctx context.Context, badge *domain.Badge) error {
	err := r.db.WithContext(ctx).Save(badge).Error
	if err != nil {
		r.l.Errorw("failed to update badge", "error", err, "id", badge.ID)
		return err
	}
	return nil
}

func (r *badgeRepository) Delete(ctx context.Context, id int) error {
	err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.Badge{}).Error
	if err != nil {
		r.l.Errorw("failed to delete badge", "error", err, "id", id)
		return err
	}
	return nil
}

func (r *badgeRepository) CountUsage(ctx context.Context, badgeID int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.ProductBadge{}).Where("badge_id = ?", badgeID).Count(&count).Error
	return count, err
}

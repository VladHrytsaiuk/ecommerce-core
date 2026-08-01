package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"gorm.io/gorm"
)

type feedbackRepository struct {
	db *gorm.DB
	l  logger.Logger
}

func NewFeedbackRepository(db *gorm.DB, l logger.Logger) domain.FeedbackRepository {
	return &feedbackRepository{db: db, l: l}
}

func (r *feedbackRepository) getDB(ctx context.Context) *gorm.DB {
	return db.GetTx(ctx, r.db).WithContext(ctx)
}

func (r *feedbackRepository) Create(ctx context.Context, f *domain.Feedback) error {
	err := r.getDB(ctx).Create(f).Error
	if err != nil {
		r.l.Errorw("failed to create feedback", "error", err)
		return err
	}
	return nil
}

func (r *feedbackRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Feedback, error) {
	var f domain.Feedback
	err := r.getDB(ctx).Where("id = ?", id).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrFeedbackNotFound
		}
		r.l.Errorw("failed to find feedback by id", "error", err, "id", id)
		return nil, err
	}
	return &f, nil
}

func (r *feedbackRepository) FindAll(ctx context.Context, pgn pagination.Params) ([]domain.Feedback, int64, error) {
	var feedbacks []domain.Feedback
	var total int64

	query := r.getDB(ctx).Model(&domain.Feedback{})

	err := query.Count(&total).Error
	if err != nil {
		r.l.Errorw("failed to count feedbacks", "error", err)
		return nil, 0, err
	}

	offset := (pgn.Page - 1) * pgn.Limit
	err = query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pgn.Limit).
		Find(&feedbacks).Error

	if err != nil {
		r.l.Errorw("failed to find feedbacks", "error", err)
		return nil, 0, err
	}

	return feedbacks, total, nil
}

package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/feedback/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/storage"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

type feedbackService struct {
	repo          domain.FeedbackRepository
	emailProvider email.Provider
	store         storage.Storage
	cfg           *config.Config
	l             logger.Logger
}

func NewFeedbackService(
	repo domain.FeedbackRepository,
	emailProvider email.Provider,
	store storage.Storage,
	cfg *config.Config,
	l logger.Logger,
) domain.FeedbackService {
	return &feedbackService{
		repo:          repo,
		emailProvider: emailProvider,
		store:         store,
		cfg:           cfg,
		l:             l,
	}
}

func (s *feedbackService) Create(
	ctx context.Context,
	feedbackType string,
	userEmail string,
	content string,
	file interface{},
	filename string,
) (*domain.Feedback, error) {
	// 1. Валідація типу
	if feedbackType != "product_improvement" && feedbackType != "bug" {
		return nil, domain.ErrInvalidFeedbackType
	}

	// 2. Завантаження медіа (якщо прикріплено)
	var mediaURL *string
	if file != nil && s.store != nil {
		url, err := s.store.Upload(ctx, file, "feedback", filename)
		if err != nil {
			s.l.Errorw("failed to upload feedback media", "error", err)
			return nil, err
		}
		mediaURL = &url
	}

	// 3. Створення запису у БД
	f := &domain.Feedback{
		ID:       uuid.New(),
		Type:     feedbackType,
		Email:    userEmail,
		Content:  content,
		MediaURL: mediaURL,
	}

	if err := s.repo.Create(ctx, f); err != nil {
		return nil, err
	}

	// 4. Асинхронне надсилання сповіщення адміністратору
	if s.cfg.AdminNotificationEmail != "" {
		// Використовуємо context.Background(), оскільки запит клієнта завершиться раніше, ніж завершиться відправка пошти
		go func(adminEmail, fType, uEmail, fContent string, mURL *string, fID uuid.UUID) {
			err := s.emailProvider.SendFeedbackEmail(adminEmail, fType, uEmail, fContent, mURL)
			if err != nil {
				s.l.Errorw("failed to send feedback email to admin (async)", "error", err, "feedback_id", fID)
			}
		}(s.cfg.AdminNotificationEmail, f.Type, f.Email, f.Content, f.MediaURL, f.ID)
	}

	return f, nil
}

func (s *feedbackService) GetByID(ctx context.Context, id uuid.UUID) (*domain.Feedback, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *feedbackService) GetList(ctx context.Context, pgn pagination.Params) ([]domain.Feedback, pagination.Metadata, error) {
	feedbacks, total, err := s.repo.FindAll(ctx, pgn)
	if err != nil {
		return nil, pagination.Metadata{}, err
	}

	meta := pagination.CalculateMetadata(total, pgn.Page, pgn.Limit)
	return feedbacks, meta, nil
}

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type sessionRepository struct {
	db     *gorm.DB
	logger logger.Logger
}

// NewSessionRepository створює новий екземпляр GORM-репозиторію для сесій.
func NewSessionRepository(db *gorm.DB, l logger.Logger) domain.SessionRepository {
	return &sessionRepository{
		db:     db,
		logger: l,
	}
}

// Create зберігає нову сесію (Refresh Token) у базі даних.
func (r *sessionRepository) Create(ctx context.Context, session *domain.Session) error {
	err := r.db.WithContext(ctx).Create(session).Error
	if err != nil {
		r.logger.Error("failed to create session",
			zap.Error(err),
			zap.String("user_id", session.UserID.String()),
		)
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// FindByToken шукає активну сесію за Refresh Token.
// Використовує Preload("User"), щоб отримати дані користувача одним запитом (Join).
func (r *sessionRepository) FindByToken(ctx context.Context, token string) (*domain.Session, error) {
	var session domain.Session
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("refresh_token = ?", token).
		First(&session).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to find session by token: %w", err)
	}
	return &session, nil
}

// DeleteByToken видаляє сесію за її токеном ( Hard Delete).
func (r *sessionRepository) DeleteByToken(ctx context.Context, token string) error {
	// Оскільки ми не маємо DeletedAt у сесії, це буде справжнє видалення.
	err := r.db.WithContext(ctx).Where("refresh_token = ?", token).Delete(&domain.Session{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete session by token: %w", err)
	}
	return nil
}

// DeleteAllForUser видаляє всі сесії конкретного користувача (напр. Logout зі всіх пристроїв).
func (r *sessionRepository) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&domain.Session{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete all sessions for user: %w", err)
	}
	return nil
}

// DeleteExpired видаляє всі прострочені сесії. Використовується для періодичного очищення БД.
func (r *sessionRepository) DeleteExpired(ctx context.Context) error {
	// Видаляємо всі сесії, де час закінчення менший за поточний.
	err := r.db.WithContext(ctx).Where("expires_at < NOW()").Delete(&domain.Session{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	return nil
}

// DeleteOldest видаляє найстаріші сесії користувача, залишаючи лише keepCount найновіших.
func (r *sessionRepository) DeleteOldest(ctx context.Context, userID uuid.UUID, keepCount int) error {
	if keepCount < 0 {
		return nil
	}

	// Знаходимо ID сесій, які потрібно ЗАЛИШИТИ (найновіші за created_at DESC)
	subQuery := r.db.Table("session").
		Select("id").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(keepCount)

	// Видаляємо всі інші сесії цього користувача.
	// GORM дозволяє використовувати підзапит у реченні NOT IN.
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND id NOT IN (?)", userID, subQuery).
		Delete(&domain.Session{}).Error

	if err != nil {
		r.logger.Error("failed to delete oldest sessions",
			zap.Error(err),
			zap.String("user_id", userID.String()),
		)
		return fmt.Errorf("failed to delete oldest sessions: %w", err)
	}

	return nil
}

package service

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/audit/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

type auditService struct {
	repo domain.AuditRepository
	log  logger.Logger
}

// NewAuditService створює новий сервіс аудиту.
func NewAuditService(repo domain.AuditRepository, log logger.Logger) domain.AuditService {
	return &auditService{
		repo: repo,
		log:  log,
	}
}

func (s *auditService) Log(ctx context.Context, log *domain.AuditLog) error {
	// Зберігаємо в базу даних
	if err := s.repo.Create(ctx, log); err != nil {
		// Якщо БД впала, ми все одно маємо лог у Zap (як fallback)
		s.log.Errorw("failed to save audit log to db", "err", err, "path", log.Path, "user_id", log.UserID)
		return err
	}
	return nil
}

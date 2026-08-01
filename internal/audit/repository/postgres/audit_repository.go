package postgres

import (
	"context"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/audit/domain"
	"gorm.io/gorm"
)

type auditRepository struct {
	db *gorm.DB
}

// NewAuditRepository створює новий екземпляр репозиторію аудиту.
func NewAuditRepository(db *gorm.DB) domain.AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) Create(ctx context.Context, log *domain.AuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

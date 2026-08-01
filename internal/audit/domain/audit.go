package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuditLog представляє запис у журналі аудиту дій адміністратора.
type AuditLog struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Method     string    `gorm:"size:10" json:"method"`
	Path       string    `gorm:"size:255" json:"path"`
	IP         string    `gorm:"size:45" json:"ip"`
	UserAgent  string    `gorm:"type:text" json:"user_agent"`
	StatusCode int       `json:"status_code"`
	Payload    string    `gorm:"type:text" json:"payload"`              // JSON тіло запиту (обмежене)
	Duration   int64     `gorm:"column:duration_ms" json:"duration_ms"` // Тривалість у мілісекундах
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

// AuditRepository визначає інтерфейс для роботи з журналом аудиту.
type AuditRepository interface {
	Create(ctx context.Context, log *AuditLog) error
	// У майбутньому можна додати List, FindByID тощо для адмінки
}

// AuditService визначає бізнес-логіку для аудиту.
type AuditService interface {
	Log(ctx context.Context, log *AuditLog) error
}

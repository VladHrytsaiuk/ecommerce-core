package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

var (
	ErrFeedbackNotFound    = errors.New("feedback not found")
	ErrInvalidFeedbackType = errors.New("invalid feedback type, must be 'product_improvement' or 'bug'")
)

type Feedback struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Type      string    `gorm:"type:varchar(50);not null" json:"type"`
	Email     string    `gorm:"type:varchar(255);not null" json:"email"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	MediaURL  *string   `gorm:"type:varchar(255)" json:"media_url,omitempty"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (Feedback) TableName() string {
	return "feedback"
}

type FeedbackRepository interface {
	Create(ctx context.Context, f *Feedback) error
	FindByID(ctx context.Context, id uuid.UUID) (*Feedback, error)
	FindAll(ctx context.Context, pgn pagination.Params) ([]Feedback, int64, error)
}

type FeedbackService interface {
	Create(ctx context.Context, feedbackType, email, content string, file interface{}, filename string) (*Feedback, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Feedback, error)
	GetList(ctx context.Context, pgn pagination.Params) ([]Feedback, pagination.Metadata, error)
}

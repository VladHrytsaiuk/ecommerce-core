package identity

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	support "github.com/VladHrytsaiuk/ecommerce-core/internal/support/domain"
)

type EmailReader struct{ db *gorm.DB }

func NewEmailReader(db *gorm.DB) *EmailReader { return &EmailReader{db: db} }
func (r *EmailReader) EmailForCustomer(ctx context.Context, id uuid.UUID) (string, error) {
	if r == nil || r.db == nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid customer email request")
	}
	var row struct{ Email string }
	if err := r.db.WithContext(ctx).Table("users").Select("email").Where("id = ?", id).Take(&row).Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(row.Email), nil
}

var _ support.CustomerEmailReader = (*EmailReader)(nil)

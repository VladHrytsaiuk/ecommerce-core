package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type UserReader struct{ db *gorm.DB }

func NewUserReader(db *gorm.DB) *UserReader { return &UserReader{db: db} }

func (r *UserReader) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var record userRecord
	if err := r.db.WithContext(ctx).Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	return &domain.User{ID: record.ID, Email: record.Email, PasswordHash: record.PasswordHash, Role: record.Role, Status: record.Status}, nil
}

type userRecord struct {
	ID           uuid.UUID `gorm:"column:id"`
	Email        string    `gorm:"column:email"`
	PasswordHash string    `gorm:"column:password_hash"`
	Role         string    `gorm:"column:role"`
	Status       string    `gorm:"column:status"`
}

func (userRecord) TableName() string { return "users" }

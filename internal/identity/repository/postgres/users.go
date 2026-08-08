package postgres

import (
	"context"

	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

type UserReader struct{ db *gorm.DB }

func NewUserReader(db *gorm.DB) *UserReader { return &UserReader{db: db} }

func (r *UserReader) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := NewUserRepository(r.db).FindByLogin(ctx, email)
	if err == domain.ErrUserNotFound {
		return nil, domain.ErrInvalidCredentials
	}
	return user, err
}

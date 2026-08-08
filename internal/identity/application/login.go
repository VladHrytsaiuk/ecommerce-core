package application

import (
	"context"
	"strings"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

type Service struct {
	users       domain.UserReader
	tokens      token.Maker
	accessToken time.Duration
}

func NewService(users domain.UserReader, tokens token.Maker, accessToken time.Duration) *Service {
	return &Service{users: users, tokens: tokens, accessToken: accessToken}
}

func (s *Service) Login(ctx context.Context, email, plainPassword string) (*domain.LoginResult, error) {
	if s.users == nil || s.tokens == nil || s.accessToken <= 0 {
		return nil, domain.ErrInvalidCredentials
	}
	user, err := s.users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || user == nil || user.Status != domain.UserStatusActive || !validRole(string(user.Role)) {
		return nil, domain.ErrInvalidCredentials
	}
	if err := password.CheckPassword(plainPassword, user.PasswordHash); err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	accessToken, claims, err := s.tokens.CreateTokenForRole(user.ID, string(user.Role), s.accessToken)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	return &domain.LoginResult{AccessToken: accessToken, ExpiresAt: claims.ExpiresAt.Time, Role: string(user.Role)}, nil
}

func validRole(role string) bool {
	switch role {
	case token.RoleCustomer, token.RoleManager, token.RoleAdmin, token.RoleOwner:
		return true
	default:
		return false
	}
}

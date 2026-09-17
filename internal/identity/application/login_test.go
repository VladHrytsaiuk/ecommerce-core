package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

func TestLoginIssuesRoleJWT(t *testing.T) {
	hash, err := password.HashPassword("a-strong-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	maker, err := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.New()
	email := "owner@example.com"
	service := NewService(fakeUsers{user: &domain.User{ID: userID, Email: &email, PasswordHash: hash, Role: token.RoleOwner, Status: domain.UserStatusActive}}, maker, time.Hour)

	result, err := service.Login(context.Background(), "OWNER@example.com", "a-strong-owner-password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	claims, err := maker.VerifyToken(result.AccessToken)
	if err != nil || claims.UserID != userID || claims.Role != token.RoleOwner {
		t.Fatalf("issued claims = (%+v, %v), want owner JWT", claims, err)
	}
}

func TestLoginDoesNotRevealInvalidCredentials(t *testing.T) {
	service := NewService(fakeUsers{err: errors.New("database unavailable")}, nil, time.Hour)
	_, err := service.Login(context.Background(), "owner@example.com", "wrong")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

type fakeUsers struct {
	user *domain.User
	err  error
}

func (f fakeUsers) FindByEmail(context.Context, string) (*domain.User, error) { return f.user, f.err }

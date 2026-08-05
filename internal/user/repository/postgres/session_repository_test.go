//go:build integration && legacy

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func TestSessionRepository_Integration(t *testing.T) {
	// 1. Setup Test DB
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	// 2. Initialize Repositories
	l := &noopLogger{}
	userRepo := NewUserRepository(gormDB, l)
	sessionRepo := NewSessionRepository(gormDB, l)
	ctx := context.Background()

	// 3. Спершу створюємо юзера, бо сесія залежить від нього (Foreign Key)
	user := &domain.User{
		ID:           uuid.New(),
		Email:        "session_owner@example.com",
		PasswordHash: "hashed",
		FirstName:    "Session",
		LastName:     "Owner",
		RoleID:       1,
	}
	err := userRepo.Create(ctx, user)
	require.NoError(t, err)

	// ==========================================
	// Test: Create Session
	// ==========================================
	agent := "Mozilla/5.0"
	ip := "127.0.0.1"
	session := &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: "valid-refresh-token",
		UserAgent:    &agent,
		ClientIP:     &ip,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	err = sessionRepo.Create(ctx, session)
	require.NoError(t, err)

	// ==========================================
	// Test: FindByToken (with Preload)
	// ==========================================
	found, err := sessionRepo.FindByToken(ctx, "valid-refresh-token")
	require.NoError(t, err)
	require.Equal(t, user.ID, found.UserID)
	require.Equal(t, user.Email, found.User.Email, "User data should be preloaded")

	// ==========================================
	// Test: DeleteByToken
	// ==========================================
	err = sessionRepo.DeleteByToken(ctx, "valid-refresh-token")
	require.NoError(t, err)

	ghost, err := sessionRepo.FindByToken(ctx, "valid-refresh-token")
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
	require.Nil(t, ghost)

	// ==========================================
	// Test: DeleteExpired
	// ==========================================
	// Створюємо живу сесію
	sessionRepo.Create(ctx, &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: "alive-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	})

	// Створюємо прострочену сесію
	sessionRepo.Create(ctx, &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: "expired-token",
		ExpiresAt:    time.Now().Add(-1 * time.Minute),
	})

	err = sessionRepo.DeleteExpired(ctx)
	require.NoError(t, err)

	// Перевіряємо, що прострочена зникла
	_, err = sessionRepo.FindByToken(ctx, "expired-token")
	require.ErrorIs(t, err, domain.ErrSessionNotFound)

	// Перевіряємо, що жива залишилася
	alive, err := sessionRepo.FindByToken(ctx, "alive-token")
	require.NoError(t, err)
	require.NotNil(t, alive)

	// ==========================================
	// Test: DeleteAllForUser
	// ==========================================
	err = sessionRepo.DeleteAllForUser(ctx, user.ID)
	require.NoError(t, err)

	alive, err = sessionRepo.FindByToken(ctx, "alive-token")
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}

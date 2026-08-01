//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func ptr[T any](v T) *T {
	return &v
}

func TestUserRepository_Integration(t *testing.T) {
	// 1. Setup Test DB
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	// 2. Initialize Repository
	l := &noopLogger{} // Тимчасовий логер для тестів
	repo := NewUserRepository(gormDB, l)
	ctx := context.Background()

	// ==========================================
	// Test: Create User
	// ==========================================
	user := &domain.User{
		ID:           uuid.New(),
		Email:        "test@example.com",
		PasswordHash: "hashed-pwd",
		FirstName:    "John",
		LastName:     "Doe",
		Phone:        ptr("+380123456789"),
		RoleID:       1,
	}

	err := repo.Create(ctx, user)
	require.NoError(t, err)
	require.NotEmpty(t, user.ID)

	// ==========================================
	// Test: Duplicate Email (Unique Constraint)
	// ==========================================
	err = repo.Create(ctx, &domain.User{
		Email:        "test@example.com", // той самий email
		PasswordHash: "pwd",
		FirstName:    "Other",
		LastName:     "User",
		Phone:        ptr("+380999999999"),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrEmailAlreadyExists)

	// ==========================================
	// Test: FindByEmail
	// ==========================================
	found, err := repo.FindByEmail(ctx, "test@example.com")
	require.NoError(t, err)
	require.Equal(t, user.FirstName, found.FirstName)
	require.Equal(t, user.ID, found.ID)

	// ==========================================
	// Test: FindByPhone
	// ==========================================
	foundPhone, err := repo.FindByPhone(ctx, "+380123456789")
	require.NoError(t, err)
	require.Equal(t, user.FirstName, foundPhone.FirstName)
	require.Equal(t, user.ID, foundPhone.ID)

	// FindByPhone — не існуючий номер
	_, err = repo.FindByPhone(ctx, "+380000000000")
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrUserNotFound)

	// ==========================================
	// Test: Update
	// ==========================================
	found.FirstName = "UpdatedName"
	err = repo.Update(ctx, found)
	require.NoError(t, err)

	updated, err := repo.FindByID(ctx, found.ID)
	require.NoError(t, err)
	require.Equal(t, "UpdatedName", updated.FirstName)

	// ==========================================
	// Test: Delete (Soft Delete)
	// ==========================================
	err = repo.Delete(ctx, user.ID)
	require.NoError(t, err)

	// Користувач не повинен бути доступний через звичайний пошук
	ghost, err := repo.FindByID(ctx, user.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrUserNotFound)
	require.Nil(t, ghost)

	// АЛЕ він має залишитися в БД (Soft Delete перевірка через прямий SQL)
	var count int64
	gormDB.Table(`"user"`).Unscoped().Where("id = ?", user.ID).Count(&count)
	require.Equal(t, int64(1), count, "User record must remain in DB due to Soft Delete")
}

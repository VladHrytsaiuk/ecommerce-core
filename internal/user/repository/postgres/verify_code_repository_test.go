//go:build integration

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

func TestVerifyCodeRepository_Integration(t *testing.T) {
	// 1. Setup Test DB
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	// 2. Initialize Repository
	l := &noopLogger{}
	userRepo := NewUserRepository(gormDB, l)
	verifyRepo := NewVerifyCodeRepository(gormDB, l)
	ctx := context.Background()

	// 3. User setup
	user := &domain.User{ID: uuid.New(), Email: "code_owner@example.com", PasswordHash: "hashed", FirstName: "Code", LastName: "Owner", RoleID: 1}
	userRepo.Create(ctx, user)

	// ==========================================
	// Test: Create Code
	// ==========================================
	codeID := uuid.New()
	code := &domain.VerifyCode{
		ID:        codeID,
		UserID:    &user.ID,
		Target:    user.Email,
		Type:      "email_verification",
		Code:      "112233",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		// Явно ставимо час у минуле, щоб перевірити сортування
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}

	err := verifyRepo.Create(ctx, code)
	require.NoError(t, err)

	// ==========================================
	// Test: FindLastByTarget
	// ==========================================
	// Створимо ще один код для тієї ж цілі. Він створиться з поточним часом,
	// тому має стати "останнім" у видачі репозиторію.
	secondCode := &domain.VerifyCode{
		ID:        uuid.New(),
		UserID:    &user.ID,
		Target:    user.Email,
		Type:      "email_verification",
		Code:      "998877",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	err = verifyRepo.Create(ctx, secondCode)
	require.NoError(t, err)

	last, err := verifyRepo.FindLastByTarget(ctx, user.Email, "email_verification")
	require.NoError(t, err)
	require.NotNil(t, last)
	require.Equal(t, "998877", last.Code, "Повинен знайти останній створений код")

	// ==========================================
	// Test: MarkAsUsed
	// ==========================================
	err = verifyRepo.MarkAsUsed(ctx, last.ID)
	require.NoError(t, err)

	updated, err := verifyRepo.FindLastByTarget(ctx, user.Email, "email_verification")
	require.NoError(t, err)
	require.True(t, updated.IsUsed)

	// ==========================================
	// Test: DeleteExpired
	// ==========================================
	// Додаємо прострочений код
	expiredCode := &domain.VerifyCode{
		ID:        uuid.New(),
		UserID:    &user.ID,
		Target:    "expired@example.com",
		Type:      "password_reset",
		Code:      "000000",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	verifyRepo.Create(ctx, expiredCode)

	err = verifyRepo.DeleteExpired(ctx)
	require.NoError(t, err)

	// Перевіряємо, що прострочений видалений
	ghost, err := verifyRepo.FindLastByTarget(ctx, "expired@example.com", "password_reset")
	require.NoError(t, err)
	require.Nil(t, ghost, "Прострочений код має зникнути")

	// А живий код (другий, який ми використали, але він не прострочений) має залишитися
	stillThere, err := verifyRepo.FindLastByTarget(ctx, user.Email, "email_verification")
	require.NoError(t, err)
	require.NotNil(t, stillThere)
}

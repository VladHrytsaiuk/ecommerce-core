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

func TestUserAddressRepository_Integration(t *testing.T) {
	// 1. Setup Test DB
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	// 2. Initialize Repositories
	l := &noopLogger{}
	userRepo := NewUserRepository(gormDB, l)
	addressRepo := NewUserAddressRepository(gormDB, l)
	ctx := context.Background()

	// 3. Спершу створюємо юзера
	user := &domain.User{
		ID:           uuid.New(),
		Email:        "address_owner@example.com",
		PasswordHash: "hashed",
		FirstName:    "Address",
		LastName:     "Owner",
		RoleID:       1,
	}
	err := userRepo.Create(ctx, user)
	require.NoError(t, err)

	// ==========================================
	// Test: Create Address
	// ==========================================
	addr1 := &domain.UserAddress{
		ID:            uuid.New(),
		UserID:        user.ID,
		Provider:      "NOVA_POSHTA",
		DeliveryType:  "BRANCH",
		FullAddress:   "м. Київ, Відділення №1",
		CityRef:       "cr1",
		CityName:      "Київ",
		WarehouseRef:  "wr1",
		WarehouseName: "№1",
		IsDefault:     true,
	}

	err = addressRepo.Create(ctx, addr1)
	require.NoError(t, err)

	// ==========================================
	// Test: UnsetDefaultAllForUser
	// ==========================================
	err = addressRepo.UnsetDefaultAllForUser(ctx, user.ID)
	require.NoError(t, err)

	found, err := addressRepo.FindByID(ctx, addr1.ID)
	require.NoError(t, err)
	require.False(t, found.IsDefault, "Адреса більше не повинна бути дефолтною")

	// ==========================================
	// Test: Atomic (Transaction)
	// ==========================================
	// Спробуємо в транзакції: скинути всі старі та створити нову дефолтну
	newAddr := &domain.UserAddress{
		ID:            uuid.New(),
		UserID:        user.ID,
		Provider:      "NOVA_POSHTA",
		DeliveryType:  "BRANCH",
		FullAddress:   "м. Львів, Відділення №5",
		CityRef:       "cr2",
		CityName:      "Львів",
		WarehouseRef:  "wr2",
		WarehouseName: "№5",
		IsDefault:     true,
	}

	err = addressRepo.Atomic(ctx, func(repo domain.UserAddressRepository) error {
		if err := repo.UnsetDefaultAllForUser(ctx, user.ID); err != nil {
			return err
		}
		return repo.Create(ctx, newAddr)
	})
	require.NoError(t, err)

	// Перевіряємо результат транзакції
	all, err := addressRepo.FindAllByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, all, 2)

	for _, a := range all {
		if a.ID == newAddr.ID {
			require.True(t, a.IsDefault, "Нова адреса має бути дефолтною")
		} else {
			require.False(t, a.IsDefault, "Стара адреса має бути НЕ дефолтною")
		}
	}

	// ==========================================
	// Test: Delete (Soft Delete)
	// ==========================================
	err = addressRepo.Delete(ctx, addr1.ID)
	require.NoError(t, err)

	// Перевіряємо, що в списку активних адрес її немає
	active, err := addressRepo.FindAllByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, newAddr.ID, active[0].ID)
}

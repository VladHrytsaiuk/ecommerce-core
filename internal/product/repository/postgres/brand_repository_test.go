//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func TestBrandRepository_Integration(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewBrandRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	// Seed Brands
	brand1ID := uuid.New()
	brand2ID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Zebra Brand')`, brand1ID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Alpha Brand')`, brand2ID).Error)

	// ==========================================
	// Test: FindAll — перевірка сортування ORDER BY name ASC
	// ==========================================
	brands, err := repo.FindAll(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(brands), 2)

	// Знаходимо індекси наших брендів у результаті
	var idxAlpha, idxZebra int
	for i, b := range brands {
		if b.ID == brand2ID {
			idxAlpha = i
		}
		if b.ID == brand1ID {
			idxZebra = i
		}
	}
	// 'Alpha Brand' повинен бути ПЕРЕД 'Zebra Brand' через ORDER BY name ASC
	assert.Less(t, idxAlpha, idxZebra, "Alpha Brand should appear before Zebra Brand in sorted list")
	assert.Equal(t, "Alpha Brand", brands[idxAlpha].Name)
	assert.Equal(t, "Zebra Brand", brands[idxZebra].Name)

	// ==========================================
	// Test: FindByID — happy path
	// ==========================================
	b, err := repo.FindByID(ctx, brand1ID)
	require.NoError(t, err)
	assert.Equal(t, brand1ID, b.ID)
	assert.Equal(t, "Zebra Brand", b.Name)

	// ==========================================
	// Test: FindByID — not found
	// ==========================================
	_, err = repo.FindByID(ctx, uuid.New())
	assert.ErrorIs(t, err, domain.ErrBrandNotFound)

	// ==========================================
	// Test: Soft-deleted brand не повертається
	// ==========================================
	require.NoError(t, gormDB.Exec(`UPDATE brand SET deleted_at = NOW() WHERE id = ?`, brand1ID).Error)

	brands2, err := repo.FindAll(ctx)
	require.NoError(t, err)
	for _, b := range brands2 {
		assert.NotEqual(t, brand1ID, b.ID, "Soft-deleted brand should not appear in FindAll")
	}

	_, err = repo.FindByID(ctx, brand1ID)
	assert.ErrorIs(t, err, domain.ErrBrandNotFound, "Soft-deleted brand should not be found by FindByID")
}

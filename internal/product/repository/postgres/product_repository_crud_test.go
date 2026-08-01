//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

func TestProductRepository_CRUD(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Test Brand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	prodID := uuid.New()
	prod := &domain.Product{
		ID:            prodID,
		BrandID:       brandID,
		CategoryID:    catID,
		IsActive:      true,
		IsBundle:      false,
		PriceStrategy: domain.PriceStrategyManual,
		Translations: []domain.ProductTranslation{
			{
				ProductID:    prodID,
				LanguageCode: "uk",
				Name:         "Тест Продукт",
				Slug:         "test-product",
				Description:  "Опис",
			},
		},
		Variations: []domain.ProductVariation{
			{
				ID:        uuid.New(),
				ProductID: prodID,
				SKU:       "TEST-SKU-1",
				Price:     1000,
				IsActive:  true,
			},
		},
	}

	// 1. Create
	err := repo.Create(ctx, prod)
	require.NoError(t, err)

	// 2. Update
	prod.IsActive = false
	prod.Translations[0].Name = "Тест Продукт Оновлено"
	err = repo.Update(ctx, prod)
	require.NoError(t, err)

	updated, err := repo.FindByID(ctx, prodID, "uk")
	require.NoError(t, err)
	assert.False(t, updated.IsActive)
	assert.Equal(t, "Тест Продукт Оновлено", updated.Translations[0].Name)

	// 3. Delete
	err = repo.Delete(ctx, prodID)
	require.NoError(t, err)

	exists, err := repo.Exists(ctx, prodID)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestProductRepository_Counts(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Count Brand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	prodID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active) VALUES (?, ?, ?, true)`, prodID, brandID, catID).Error)

	countB, err := repo.CountByBrand(ctx, brandID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countB)

	countC, err := repo.CountByCategory(ctx, catID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countC)
}

func TestProductRepository_GetActiveCategoryIDs(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID1 := uuid.New()
	catID2 := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Brand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 1)`, catID1).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 2)`, catID2).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active) VALUES (?, ?, ?, true)`, uuid.New(), brandID, catID1).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active) VALUES (?, ?, ?, false)`, uuid.New(), brandID, catID2).Error) // inactive

	catIDs, err := repo.GetActiveCategoryIDs(ctx)
	require.NoError(t, err)
	assert.True(t, catIDs[catID1])
	assert.False(t, catIDs[catID2])
}

func TestProductRepository_Reviews_CRUD(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	prodID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name, slug) VALUES (?, 'Brand', 'brand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 1)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active) VALUES (?, ?, ?, true)`, prodID, brandID, catID).Error)

	userID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO "user" (id, email, password_hash, role_id) VALUES (?, 'crud@test.com', 'pwd', 1)`, userID).Error)

	reviewID := uuid.New()
	review := &domain.ProductReview{
		ID:        reviewID,
		ProductID: prodID,
		UserID:    userID,
		Rating:    5,
		Comment:   "Good",
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	// CreateReview
	err := repo.CreateReview(ctx, review)
	require.NoError(t, err)

	// FindPendingReviews
	pending, total, err := repo.FindPendingReviews(ctx, pagination.Params{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, pending, 1)
	assert.Equal(t, reviewID, pending[0].ID)

	// ApproveReview
	err = repo.ApproveReview(ctx, reviewID)
	require.NoError(t, err)

	pending2, total2, err := repo.FindPendingReviews(ctx, pagination.Params{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total2)
	assert.Empty(t, pending2)
}

func TestProductRepository_QuickSearch(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	prodID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'Brand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 1)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active) VALUES (?, ?, ?, true)`, prodID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name, slug) VALUES (?, 'uk', 'Крутий товар', 'krutyi-tovar')`, prodID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'SKU123', 1000, true)`, uuid.New(), prodID).Error)

	res, err := repo.QuickSearch(ctx, "крут", "uk", 10)
	require.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, "krutyi-tovar", res[0].Slug)
}

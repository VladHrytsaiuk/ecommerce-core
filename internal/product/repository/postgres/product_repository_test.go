//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

func TestProductRepository_Exists(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	prodActiveID := uuid.New()
	prodInactiveID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'B')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-active')`, prodActiveID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, false, 'prod-inactive')`, prodInactiveID, brandID, catID).Error)

	// Активний товар — знайдено
	exists, err := repo.Exists(ctx, prodActiveID)
	require.NoError(t, err)
	assert.True(t, exists)

	// Неактивний товар — НЕ знайдено (is_active = false)
	exists, err = repo.Exists(ctx, prodInactiveID)
	require.NoError(t, err)
	assert.False(t, exists, "Inactive product should not be found by Exists")

	// Неіснуючий UUID — НЕ знайдено
	exists, err = repo.Exists(ctx, uuid.New())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestProductRepository_FindByID(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	prodID := uuid.New()
	varActiveID := uuid.New()
	varInactiveID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'TestBrand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-test')`, prodID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name, description) VALUES (?, 'uk', 'Шампунь', 'Опис шампуню')`, prodID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'ACTIVE-SKU', 10000, true)`, varActiveID, prodID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'INACTIVE-SKU', 5000, false)`, varInactiveID, prodID).Error)

	// Happy path — Preload перевірка
	prod, err := repo.FindByID(ctx, prodID, "uk")
	require.NoError(t, err)
	assert.Equal(t, prodID, prod.ID)

	// Переклад завантажений
	require.Len(t, prod.Translations, 1)
	assert.Equal(t, "Шампунь", prod.Translations[0].Name)
	assert.Equal(t, "Опис шампуню", prod.Translations[0].Description)

	// Бренд завантажений
	assert.Equal(t, brandID, prod.Brand.ID)
	assert.Equal(t, "TestBrand", prod.Brand.Name)

	// Тільки активні варіації повертаються
	require.Len(t, prod.Variations, 1, "Only active variations should be preloaded")
	assert.Equal(t, "ACTIVE-SKU", prod.Variations[0].SKU)

	// Неіснуючий ID
	_, err = repo.FindByID(ctx, uuid.New(), "uk")
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	// Неактивний продукт не знаходиться
	inactiveProdID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, false, 'prod-inactive-test')`, inactiveProdID, brandID, catID).Error)
	_, err = repo.FindByID(ctx, inactiveProdID, "uk")
	assert.ErrorIs(t, err, domain.ErrProductNotFound, "Inactive product should return ErrProductNotFound")
}

func TestProductRepository_Reviews(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	userID := uuid.New()
	brandID := uuid.New()
	catID := uuid.New()
	prodID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO "user" (id, email, password_hash, role_id) VALUES (?, 'rev@test.com', 'pwd', 1)`, userID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name, slug) VALUES (?, 'B', 'b')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, average_rating, reviews_count) VALUES (?, ?, ?, true, 0, 0)`, prodID, brandID, catID).Error)

	// ==========================================
	// CreateReview — happy path
	// ==========================================
	review1 := &domain.ProductReview{
		ID:        uuid.New(),
		ProductID: prodID,
		UserID:    userID,
		Rating:    5,
		Comment:   "Excellent!",
	}
	require.NoError(t, repo.CreateReview(ctx, review1))

	review2 := &domain.ProductReview{
		ID:        uuid.New(),
		ProductID: prodID,
		UserID:    userID,
		Rating:    3,
		Comment:   "Average",
	}
	require.NoError(t, repo.CreateReview(ctx, review2))

	// ==========================================
	// FindReviews — неодобрені відгуки НЕ повертаються
	// ==========================================
	reviews, count, err := repo.FindReviews(ctx, prodID, pagination.Params{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.Empty(t, reviews)

	// ==========================================
	// ApproveReview — оновлює is_approved + перераховує статистику
	// ==========================================
	require.NoError(t, repo.ApproveReview(ctx, review1.ID))

	// Перевіряємо, що product.average_rating та reviews_count оновилися
	var avgRating float64
	var reviewsCount int
	require.NoError(t, gormDB.Raw(`SELECT average_rating, reviews_count FROM product WHERE id = ?`, prodID).Row().Scan(&avgRating, &reviewsCount))
	assert.Equal(t, float64(5), avgRating, "Average rating should be 5 after approving one 5-star review")
	assert.Equal(t, 1, reviewsCount, "Reviews count should be 1")

	// Тепер схвалюємо другий відгук
	require.NoError(t, repo.ApproveReview(ctx, review2.ID))

	require.NoError(t, gormDB.Raw(`SELECT average_rating, reviews_count FROM product WHERE id = ?`, prodID).Row().Scan(&avgRating, &reviewsCount))
	assert.Equal(t, float64(4), avgRating, "Average rating should be 4 after 5-star + 3-star reviews")
	assert.Equal(t, 2, reviewsCount, "Reviews count should be 2")

	// Додаємо відповідь на відгук (рейтинг 0, parent_id != nil)
	reply1 := &domain.ProductReview{
		ID:        uuid.New(),
		ProductID: prodID,
		UserID:    userID,
		ParentID:  &review1.ID,
		Rating:    0,
		Comment:   "Дякуємо за відгук!",
	}
	require.NoError(t, repo.CreateReview(ctx, reply1))
	require.NoError(t, repo.ApproveReview(ctx, reply1.ID))

	// Перевіряємо, що після додавання та схвалення відповіді рейтинг та кількість відгуків не змінилися
	require.NoError(t, gormDB.Raw(`SELECT average_rating, reviews_count FROM product WHERE id = ?`, prodID).Row().Scan(&avgRating, &reviewsCount))
	assert.Equal(t, float64(4), avgRating, "Average rating should remain 4 after approving a reply")
	assert.Equal(t, 2, reviewsCount, "Reviews count should remain 2 after approving a reply")

	// ==========================================
	// FindReviews — одобрені відгуки та відповіді повертаються з правильною пагінацією
	// ==========================================
	reviews, count, err = repo.FindReviews(ctx, prodID, pagination.Params{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.Len(t, reviews, 3)

	// Перевіряємо пагінацію — limit = 2
	reviews, count, err = repo.FindReviews(ctx, prodID, pagination.Params{Page: 1, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count, "Total count should still be 3 regardless of limit")
	assert.Len(t, reviews, 2, "Only 2 reviews returned due to limit")

	// Сторінка 2
	reviews, count, err = repo.FindReviews(ctx, prodID, pagination.Params{Page: 2, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, reviews, 1, "Second page should have 1 review")

	// ==========================================
	// RejectReview — відхиляємо вже схвалений відгук
	// ==========================================
	reason := "Spam"
	require.NoError(t, repo.RejectReview(ctx, review1.ID, &reason))

	// Перевіряємо, що рейтинг перерахувався (залишився тільки review2 з рейтингом 3)
	require.NoError(t, gormDB.Raw(`SELECT average_rating, reviews_count FROM product WHERE id = ?`, prodID).Row().Scan(&avgRating, &reviewsCount))
	assert.Equal(t, float64(3), avgRating, "Average rating should be 3 after rejecting the 5-star review")
	assert.Equal(t, 1, reviewsCount, "Reviews count should be 1 after rejecting a review")

	// Перевіряємо, що відхилений відгук не повертається у FindReviews
	reviews, count, err = repo.FindReviews(ctx, prodID, pagination.Params{Page: 1, Limit: 10})
	require.NoError(t, err)
	// count=2 (один відгук + одна відповідь на відхилений відгук)
	// але ми відхилили батьківський відгук. У нас є review2 (rating 3) і reply1. 
	assert.Equal(t, int64(2), count, "Count should drop to 2 (review2 + reply1)")
	for _, r := range reviews {
		assert.NotEqual(t, review1.ID, r.ID, "Rejected review should not be returned in public list")
	}

	// Відхилений відгук НЕ повертається у PendingReviews
	pendingReviews, pendingCount, err := repo.FindPendingReviews(ctx, pagination.Params{Page: 1, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(0), pendingCount)
	assert.Empty(t, pendingReviews)
}

func TestProductRepository_FindAll_Filters(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	// Seed: 2 бренди, 2 категорії, 2 товари з різними цінами
	brandA := uuid.New()
	brandB := uuid.New()
	catA := uuid.New()
	catB := uuid.New()
	prodCheap := uuid.New()
	prodExpensive := uuid.New()
	varCheap := uuid.New()
	varExpensive := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'BrandA')`, brandA).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'BrandB')`, brandB).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catA).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 1)`, catB).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-cheap')`, prodCheap, brandA, catA).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-expensive')`, prodExpensive, brandB, catB).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Дешевий')`, prodCheap).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Дорогий')`, prodExpensive).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'CHEAP', 5000, true)`, varCheap, prodCheap).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'EXP', 50000, true)`, varExpensive, prodExpensive).Error)

	defaultPgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	// ==========================================
	// Без фільтрів — повертаються обидва
	// ==========================================
	variations, count, err := repo.FindAll(ctx, "uk", domain.ProductFilter{}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.Len(t, variations, 2)

	// ==========================================
	// Фільтр за CategoryID
	// ==========================================
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		CategoryID: []uuid.UUID{catA},
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.Len(t, variations, 1)
	assert.Equal(t, varCheap, variations[0].ID)

	// ==========================================
	// Фільтр за BrandID
	// ==========================================
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		BrandID: []uuid.UUID{brandB},
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.Len(t, variations, 1)
	assert.Equal(t, varExpensive, variations[0].ID)

	// ==========================================
	// Фільтр за MinPrice
	// ==========================================
	minPrice := 10000
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		MinPrice: &minPrice,
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.Len(t, variations, 1)
	assert.Equal(t, 50000, variations[0].Price)

	// ==========================================
	// Фільтр за MaxPrice
	// ==========================================
	maxPrice := 10000
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		MaxPrice: &maxPrice,
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.Len(t, variations, 1)
	assert.Equal(t, 5000, variations[0].Price)

	// ==========================================
	// Фільтр за діапазоном ціни (MinPrice + MaxPrice)
	// ==========================================
	minP, maxP := 1000, 6000
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		MinPrice: &minP,
		MaxPrice: &maxP,
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Equal(t, 5000, variations[0].Price)

	// Діапазон, що не включає жодного товару
	minP2, maxP2 := 100000, 200000
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		MinPrice: &minP2,
		MaxPrice: &maxP2,
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.Empty(t, variations)

	// ==========================================
	// Комбінований фільтр: Brand + MaxPrice
	// ==========================================
	variations, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{
		BrandID:  []uuid.UUID{brandA},
		MaxPrice: &maxPrice, // 10000
	}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Equal(t, "CHEAP", variations[0].SKU)
}

func TestProductRepository_FindAll_Sorting(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	prod1 := uuid.New()
	prod2 := uuid.New()
	var1 := uuid.New()
	var2 := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'B')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, average_rating, slug) VALUES (?, ?, ?, true, 4.5, 'prod-juice-apple')`, prod1, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, average_rating, slug) VALUES (?, ?, ?, true, 2.0, 'prod-juice-orange')`, prod2, brandID, catID).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Яблучний сік')`, prod1).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Апельсиновий сік')`, prod2).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'CHEAP', 5000, true)`, var1, prod1).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'EXP', 50000, true)`, var2, prod2).Error)

	// ==========================================
	// Сортування за ціною ASC
	// ==========================================
	variations, _, err := repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 1, Limit: 10, SortBy: "price", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, variations, 2)
	assert.Equal(t, 5000, variations[0].Price, "Cheapest first with price ASC")
	assert.Equal(t, 50000, variations[1].Price)

	// ==========================================
	// Сортування за ціною DESC
	// ==========================================
	variations, _, err = repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 1, Limit: 10, SortBy: "price", Order: "desc"})
	require.NoError(t, err)
	require.Len(t, variations, 2)
	assert.Equal(t, 50000, variations[0].Price, "Most expensive first with price DESC")
	assert.Equal(t, 5000, variations[1].Price)

	// ==========================================
	// Сортування за рейтингом DESC
	// ==========================================
	variations, _, err = repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 1, Limit: 10, SortBy: "rating", Order: "desc"})
	require.NoError(t, err)
	require.Len(t, variations, 2)
	assert.Equal(t, var1, variations[0].ID, "Highest rated product should be first with rating DESC")

	// ==========================================
	// Сортування за назвою ASC (uk)
	// ==========================================
	variations, _, err = repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 1, Limit: 10, SortBy: "name", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, variations, 2)
	// "Апельсиновий сік" < "Яблучний сік" в UTF8
	assert.Equal(t, var2, variations[0].ID, "Alphabetically first product should be first with name ASC")

	// ==========================================
	// Пагінація
	// ==========================================
	variations, count, err := repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 1, Limit: 1, SortBy: "price", Order: "asc"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), count, "Total count must be 2 regardless of limit")
	assert.Len(t, variations, 1)
	assert.Equal(t, 5000, variations[0].Price)

	// Сторінка 2
	variations, _, err = repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 2, Limit: 1, SortBy: "price", Order: "asc"})
	require.NoError(t, err)
	assert.Len(t, variations, 1)
	assert.Equal(t, 50000, variations[0].Price)
}

func TestProductRepository_FindAll_Preloads(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	prodID := uuid.New()
	varID := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'PreloadBrand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-preload')`, prodID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Тестовий товар')`, prodID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'V1', 15000, true)`, varID, prodID).Error)

	// Додаємо атрибут
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute (id, code, is_filterable, is_variant_specific) VALUES (100, 'size', true, false)`).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute_translation (attribute_id, language_code, name) VALUES (100, 'uk', 'Розмір')`).Error)
	attrJSON, _ := json.Marshal(map[string]string{"uk": "Великий"})
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute_value (id, attribute_id, product_id, value_string) VALUES (?, 100, ?, ?)`, uuid.New(), prodID, string(attrJSON)).Error)

	variations, _, err := repo.FindAll(ctx, "uk", domain.ProductFilter{}, pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"})
	require.NoError(t, err)
	require.Len(t, variations, 1)

	v := variations[0]

	// Product preloaded
	assert.Equal(t, prodID, v.Product.ID)

	// Product.Brand preloaded
	assert.Equal(t, "PreloadBrand", v.Product.Brand.Name)

	// Product.Translations preloaded (filtered by lang)
	require.Len(t, v.Product.Translations, 1)
	assert.Equal(t, "Тестовий товар", v.Product.Translations[0].Name)
}

func TestProductRepository_GetFilters_Deep(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandA := uuid.New()
	brandB := uuid.New()
	catID := uuid.New()
	prod1 := uuid.New()
	prod2 := uuid.New()
	var1 := uuid.New()
	var2 := uuid.New()

	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'FilterBrandA')`, brandA).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'FilterBrandB')`, brandB).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-color-red')`, prod1, brandA, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug) VALUES (?, ?, ?, true, 'prod-color-blue')`, prod2, brandB, catID).Error)

	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'F1', 7000, true)`, var1, prod1).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'F2', 25000, true)`, var2, prod2).Error)

	// Атрибут "color" з перекладом
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute (id, code, is_filterable, is_variant_specific) VALUES (200, 'color', true, false)`).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute_translation (attribute_id, language_code, name) VALUES (200, 'uk', 'Колір')`).Error)

	colorRed, _ := json.Marshal(map[string]string{"uk": "Червоний"})
	colorBlue, _ := json.Marshal(map[string]string{"uk": "Синій"})
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute_value (id, attribute_id, product_id, value_string) VALUES (?, 200, ?, ?)`, uuid.New(), prod1, string(colorRed)).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO attribute_value (id, attribute_id, product_id, value_string) VALUES (?, 200, ?, ?)`, uuid.New(), prod2, string(colorBlue)).Error)

	// ==========================================
	// GetFilters без фільтрів — повна discovery
	// ==========================================
	filters, err := repo.GetFilters(ctx, domain.ProductFilter{}, "uk")
	require.NoError(t, err)
	require.NotNil(t, filters)

	// Ціни
	assert.Equal(t, 7000, filters.MinPrice)
	assert.Equal(t, 25000, filters.MaxPrice)

	// Бренди
	require.GreaterOrEqual(t, len(filters.Brands), 2)
	brandNames := make(map[string]int)
	for _, b := range filters.Brands {
		brandNames[b.Name] = b.Count
	}
	assert.Equal(t, 1, brandNames["FilterBrandA"])
	assert.Equal(t, 1, brandNames["FilterBrandB"])

	// Атрибути
	require.GreaterOrEqual(t, len(filters.Attributes), 1)
	var colorAttr *domain.AttributeFilter
	for i, a := range filters.Attributes {
		if a.Code == "color" {
			colorAttr = &filters.Attributes[i]
			break
		}
	}
	require.NotNil(t, colorAttr, "Color attribute should be in filters")
	assert.Equal(t, "Колір", colorAttr.Name, "Attribute name should be localized")
	assert.GreaterOrEqual(t, len(colorAttr.Values), 2)

	colorValues := make(map[string]int)
	for _, v := range colorAttr.Values {
		colorValues[v.Label] = v.Count
	}
	assert.Equal(t, 1, colorValues["Червоний"])
	assert.Equal(t, 1, colorValues["Синій"])

	// ==========================================
	// GetFilters з фільтром CategoryID
	// ==========================================
	otherCat := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 1)`, otherCat).Error)

	filters, err = repo.GetFilters(ctx, domain.ProductFilter{CategoryID: []uuid.UUID{otherCat}}, "uk")
	require.NoError(t, err)
	assert.Equal(t, 0, filters.MinPrice, "No products in this category, price should be 0")
	assert.Equal(t, 0, filters.MaxPrice)
	assert.Empty(t, filters.Brands)
}

func TestProductRepository_AreVariationsNonBundle(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'TestBrand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	// Standard Product & Variation
	prodStdID := uuid.New()
	varStdID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug, is_bundle) VALUES (?, ?, ?, true, 'prod-std', false)`, prodStdID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'STD-SKU', 10000, true)`, varStdID, prodStdID).Error)

	// Bundle Product & Variation
	prodBundleID := uuid.New()
	varBundleID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug, is_bundle) VALUES (?, ?, ?, true, 'prod-bundle', true)`, prodBundleID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'BUNDLE-SKU', 10000, true)`, varBundleID, prodBundleID).Error)

	// Test variations non-bundle check
	ok, err := repo.AreVariationsNonBundle(ctx, []uuid.UUID{varStdID})
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = repo.AreVariationsNonBundle(ctx, []uuid.UUID{varBundleID})
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = repo.AreVariationsNonBundle(ctx, []uuid.UUID{varStdID, varBundleID})
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = repo.AreVariationsNonBundle(ctx, []uuid.UUID{})
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestProductRepository_BundlesCRUD(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'TestBrand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	// Standard variations to bundle
	prod1ID := uuid.New()
	var1ID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug, is_bundle) VALUES (?, ?, ?, true, 'prod-1', false)`, prod1ID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'V1', 15000, true)`, var1ID, prod1ID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Товар 1')`, prod1ID).Error)

	prod2ID := uuid.New()
	var2ID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug, is_bundle) VALUES (?, ?, ?, true, 'prod-2', false)`, prod2ID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'V2', 20000, true)`, var2ID, prod2ID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_translation (product_id, language_code, name) VALUES (?, 'uk', 'Товар 2')`, prod2ID).Error)

	// 1. Create Bundle
	bundleProductID := uuid.New()
	bundleVarID := uuid.New()
	bundle := &domain.Product{
		ID:            bundleProductID,
		BrandID:       brandID,
		CategoryID:    catID,
		IsActive:      true,
		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
		Translations: []domain.ProductTranslation{
			{ProductID: bundleProductID, LanguageCode: "uk", Name: "Мій набір", Slug: "super-bundle"},
		},
		Variations: []domain.ProductVariation{
			{ID: bundleVarID, ProductID: bundleProductID, SKU: "BUNDLE-V", Price: 30000, IsActive: true},
		},
		BundleItems: []domain.ProductBundleItem{
			{BundleID: bundleProductID, VariationID: var1ID, Quantity: 2},
			{BundleID: bundleProductID, VariationID: var2ID, Quantity: 1},
		},
	}

	err := repo.Create(ctx, bundle)
	require.NoError(t, err)

	// 2. Read Bundle and verify items are preloaded with variation and product details
	loaded, err := repo.FindByID(ctx, bundleProductID, "uk")
	require.NoError(t, err)
	assert.True(t, loaded.IsBundle)
	assert.Equal(t, domain.PriceStrategyManual, loaded.PriceStrategy)
	require.Len(t, loaded.BundleItems, 2)

	// Verify preloaded details
	var item1, item2 *domain.ProductBundleItem
	for i := range loaded.BundleItems {
		if loaded.BundleItems[i].VariationID == var1ID {
			item1 = &loaded.BundleItems[i]
		} else if loaded.BundleItems[i].VariationID == var2ID {
			item2 = &loaded.BundleItems[i]
		}
	}
	require.NotNil(t, item1)
	require.NotNil(t, item2)

	assert.Equal(t, 2, item1.Quantity)
	assert.Equal(t, 15000, item1.Variation.Price)
	assert.Equal(t, "Товар 1", item1.Variation.Product.Translations[0].Name)

	assert.Equal(t, 1, item2.Quantity)
	assert.Equal(t, 20000, item2.Variation.Price)
	assert.Equal(t, "Товар 2", item2.Variation.Product.Translations[0].Name)

	// 3. Update Bundle (change strategy and items)
	bundle.PriceStrategy = domain.PriceStrategyDynamic
	bundle.BundleItems = []domain.ProductBundleItem{
		{BundleID: bundleProductID, VariationID: var1ID, Quantity: 3},
	}

	err = repo.Update(ctx, bundle)
	require.NoError(t, err)

	// Read again and verify changes
	loaded2, err := repo.FindBySlug(ctx, "super-bundle", "uk")
	require.NoError(t, err)
	assert.Equal(t, domain.PriceStrategyDynamic, loaded2.PriceStrategy)
	require.Len(t, loaded2.BundleItems, 1)
	assert.Equal(t, var1ID, loaded2.BundleItems[0].VariationID)
	assert.Equal(t, 3, loaded2.BundleItems[0].Quantity)
}

func TestProductRepository_BundleActiveStateFiltering(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'TestBrand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	// Standard Variations
	prod1ID := uuid.New()
	var1ID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug, is_bundle) VALUES (?, ?, ?, true, 'prod-1', false)`, prod1ID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'V1', 15000, true)`, var1ID, prod1ID).Error)

	prod2ID := uuid.New()
	var2ID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO product (id, brand_id, category_id, is_active, slug, is_bundle) VALUES (?, ?, ?, true, 'prod-2', false)`, prod2ID, brandID, catID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_variation (id, product_id, sku, price, is_active) VALUES (?, ?, 'V2', 20000, true)`, var2ID, prod2ID).Error)

	// Create Bundle
	bundleProductID := uuid.New()
	bundleVarID := uuid.New()
	bundle := &domain.Product{
		ID:            bundleProductID,
		BrandID:       brandID,
		CategoryID:    catID,
		IsActive:      true,
		IsBundle:      true,
		PriceStrategy: domain.PriceStrategyManual,
		Translations: []domain.ProductTranslation{
			{ProductID: bundleProductID, LanguageCode: "uk", Name: "Набір фільтр", Slug: "super-bundle-filtering"},
		},
		Variations: []domain.ProductVariation{
			{ID: bundleVarID, ProductID: bundleProductID, SKU: "BUNDLE-FIL", Price: 30000, IsActive: true},
		},
		BundleItems: []domain.ProductBundleItem{
			{BundleID: bundleProductID, VariationID: var1ID, Quantity: 1},
			{BundleID: bundleProductID, VariationID: var2ID, Quantity: 1},
		},
	}
	err := repo.Create(ctx, bundle)
	require.NoError(t, err)

	defaultPgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	// 1. All components are active -> Bundle is returned in FindAll
	vars, count, err := repo.FindAll(ctx, "uk", domain.ProductFilter{}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count) // V1, V2, BUNDLE-FIL
	assert.Len(t, vars, 3)

	var foundBundle bool
	for _, v := range vars {
		if v.ID == bundleVarID {
			foundBundle = true
			assert.True(t, v.Product.IsBundle)
			require.Len(t, v.Product.BundleItems, 2)
			var vIDs []uuid.UUID
			for _, item := range v.Product.BundleItems {
				vIDs = append(vIDs, item.VariationID)
			}
			assert.Contains(t, vIDs, var1ID)
			assert.Contains(t, vIDs, var2ID)
		}
	}
	assert.True(t, foundBundle)

	// FindByID of bundle works
	_, err = repo.FindByID(ctx, bundleProductID, "uk")
	require.NoError(t, err)

	// 2. Mark one component variation as inactive -> Bundle should NOT be returned
	require.NoError(t, gormDB.Exec(`UPDATE product_variation SET is_active = false WHERE id = ?`, var1ID).Error)

	vars, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count) // V1 is inactive, Bundle is inactive due to V1, only V2 remains active
	assert.Len(t, vars, 1)
	assert.Equal(t, var2ID, vars[0].ID)

	// FindByID of bundle should fail with NotFound because a component variation is inactive
	_, err = repo.FindByID(ctx, bundleProductID, "uk")
	assert.ErrorIs(t, err, domain.ErrProductNotFound)

	// 3. Mark component variation active again, but mark component product inactive -> Bundle should NOT be returned
	require.NoError(t, gormDB.Exec(`UPDATE product_variation SET is_active = true WHERE id = ?`, var1ID).Error)
	require.NoError(t, gormDB.Exec(`UPDATE product SET is_active = false WHERE id = ?`, prod2ID).Error)

	vars, count, err = repo.FindAll(ctx, "uk", domain.ProductFilter{}, defaultPgn)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count) // V1 is active, V2 is active but its product is inactive, Bundle is inactive due to prod2, only V1 remains active
	assert.Len(t, vars, 1)
	assert.Equal(t, var1ID, vars[0].ID)

	// FindByID of bundle should fail with NotFound because a component product is inactive
	_, err = repo.FindByID(ctx, bundleProductID, "uk")
	assert.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestProductRepository_VariationSlugs(t *testing.T) {
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewProductRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	brandID := uuid.New()
	catID := uuid.New()
	require.NoError(t, gormDB.Exec(`INSERT INTO brand (id, name) VALUES (?, 'SlugBrand')`, brandID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, catID).Error)

	// Create unit in DB (e.g. pieces / шт)
	unitID := 99
	require.NoError(t, gormDB.Exec(`INSERT INTO unit (id, name, short_name) VALUES (?, '{"uk": "штуки"}', '{"uk": "шт"}')`, unitID).Error)

	// Insert test badge
	badgeID := 42
	require.NoError(t, gormDB.Exec(`INSERT INTO badge (id, name, color_hex) VALUES (?, '{"uk": "Хіт"}', '#FFF')`, badgeID).Error)

	prodID := uuid.New()
	varID1 := uuid.New()
	varID2 := uuid.New()
	p := &domain.Product{
		ID:         prodID,
		BrandID:    brandID,
		CategoryID: catID,
		IsActive:   true,
		Translations: []domain.ProductTranslation{
			{LanguageCode: "uk", Name: "Продукт з слагом", Slug: "sluggy-product"},
		},
		Variations: []domain.ProductVariation{
			{
				ID:            varID1,
				ProductID:     prodID,
				SKU:           "SLG-VAR-1",
				Price:         1000,
				QuantityValue: 10,
				UnitID:        &unitID,
				IsActive:      true,
			},
			{
				ID:            varID2,
				ProductID:     prodID,
				SKU:           "SLG-VAR-2",
				Price:         2000,
				QuantityValue: 20,
				UnitID:        &unitID,
				IsActive:      true,
			},
		},
	}

	err := repo.Create(ctx, p)
	require.NoError(t, err)

	// Insert product_badge associations manually to avoid GORM association auto-save conflict issues
	require.NoError(t, gormDB.Exec(`INSERT INTO product_badge (product_id, badge_id) VALUES (?, ?)`, prodID, badgeID).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO product_badge (variation_id, badge_id) VALUES (?, ?)`, varID1, badgeID).Error)

	// Verify slugs were generated correctly in Create!
	assert.Equal(t, "sluggy-product-10-sht", p.Variations[0].Slug)
	assert.Equal(t, "sluggy-product-20-sht", p.Variations[1].Slug)

	// Try loading by variation slug "sluggy-product-20-sht"
	loaded, err := repo.FindBySlug(ctx, "sluggy-product-20-sht", "uk")
	require.NoError(t, err)
	assert.Equal(t, p.ID, loaded.ID)

	// Verify badges are loaded when finding by variation slug
	loadedByVarSlug, err := repo.FindBySlug(ctx, "sluggy-product-10-sht", "uk")
	require.NoError(t, err)
	assert.Len(t, loadedByVarSlug.Badges, 1)
	assert.Equal(t, badgeID, loadedByVarSlug.Badges[0].BadgeID)
	assert.Equal(t, "Хіт", loadedByVarSlug.Badges[0].Badge.Name["uk"])

	// Find the loaded variation with badge and assert
	var loadedVar1 *domain.ProductVariation
	for i := range loadedByVarSlug.Variations {
		if loadedByVarSlug.Variations[i].ID == varID1 {
			loadedVar1 = &loadedByVarSlug.Variations[i]
		}
	}
	require.NotNil(t, loadedVar1)
	require.Len(t, loadedVar1.Badges, 1)
	assert.Equal(t, badgeID, loadedVar1.Badges[0].BadgeID)
	assert.Equal(t, "Хіт", loadedVar1.Badges[0].Badge.Name["uk"])

	// Verify badges are loaded when finding by product slug
	loadedByProdSlug, err := repo.FindBySlug(ctx, "sluggy-product", "uk")
	require.NoError(t, err)
	assert.Len(t, loadedByProdSlug.Badges, 1)
	assert.Equal(t, badgeID, loadedByProdSlug.Badges[0].BadgeID)

	// Clean update verification
	p.Translations[0].Slug = "updated-sluggy"
	err = repo.Update(ctx, p)
	require.NoError(t, err)

	assert.Equal(t, "updated-sluggy-10-sht", p.Variations[0].Slug)
	assert.Equal(t, "updated-sluggy-20-sht", p.Variations[1].Slug)
}


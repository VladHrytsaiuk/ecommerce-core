//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
)

func TestCategoryRepository_Integration(t *testing.T) {
	// 1. Setup Test DB
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	// 2. Initialize Repository
	repo := NewCategoryRepository(gormDB, &noopLogger{})
	ctx := context.Background()

	// 3. Seed Data (We don't know migration data exactly, so we insert our test case)
	parentID := uuid.New()
	childID := uuid.New()

	// Helper to insert to DB skipping foreign key checks for test simplicity (unless migration has specific rules)
	gormDB.Exec(`INSERT INTO category (id) VALUES (?)`, parentID)
	gormDB.Exec(`INSERT INTO category (id, parent_id) VALUES (?, ?)`, childID, parentID)

	gormDB.Exec(`INSERT INTO category_translation (category_id, language_code, name) VALUES (?, 'uk', 'Косметика')`, parentID)
	gormDB.Exec(`INSERT INTO category_translation (category_id, language_code, name) VALUES (?, 'en', 'Cosmetics')`, parentID)

	gormDB.Exec(`INSERT INTO category_translation (category_id, language_code, name) VALUES (?, 'uk', 'Шампуні')`, childID)

	// ==========================================
	// Test: FindAll (filtering by language)
	// ==========================================
	categories, err := repo.FindAll(ctx, "uk")
	require.NoError(t, err)

	// Will have at least our 2
	assert.GreaterOrEqual(t, len(categories), 2)

	// Ensure language preloading
	foundParent := false
	for _, c := range categories {
		if c.ID == parentID {
			foundParent = true
			require.Len(t, c.Translations, 1)
			assert.Equal(t, "Косметика", c.Translations[0].Name)
		}
	}
	assert.True(t, foundParent)

	// ==========================================
	// Test: FindByID
	// ==========================================
	cat, err := repo.FindByID(ctx, childID, "uk")
	require.NoError(t, err)
	assert.Equal(t, childID, cat.ID)
	assert.Equal(t, parentID, *cat.ParentID)
	require.Len(t, cat.Translations, 1)
	assert.Equal(t, "Шампуні", cat.Translations[0].Name)

	catEn, err := repo.FindByID(ctx, parentID, "en")
	require.NoError(t, err)
	require.Len(t, catEn.Translations, 1)
	assert.Equal(t, "Cosmetics", catEn.Translations[0].Name)

	catAll, err := repo.FindByID(ctx, parentID, "all")
	require.NoError(t, err)
	assert.Len(t, catAll.Translations, 2)

	// NotFound
	_, err = repo.FindByID(ctx, uuid.New(), "uk")
	assert.ErrorIs(t, err, domain.ErrCategoryNotFound)

	// ==========================================
	// Test: Sort Order & Uniqueness
	// ==========================================
	// Create another root category with sort_order = 1
	root2ID := uuid.New()
	err = gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 1)`, root2ID).Error
	require.NoError(t, err)

	// Trying to insert another root category with sort_order = 0 should fail due to uniqueness index idx_category_root_sort_order
	rootConflictID := uuid.New()
	err = gormDB.Exec(`INSERT INTO category (id, sort_order) VALUES (?, 0)`, rootConflictID).Error
	assert.Error(t, err)

	// Try to insert a subcategory with sort_order = 1 under parentID
	child2ID := uuid.New()
	err = gormDB.Exec(`INSERT INTO category (id, parent_id, sort_order) VALUES (?, ?, 1)`, child2ID, parentID).Error
	require.NoError(t, err)

	// Trying to insert another subcategory with sort_order = 0 under parentID should fail due to idx_category_parent_sort_order
	childConflictID := uuid.New()
	err = gormDB.Exec(`INSERT INTO category (id, parent_id, sort_order) VALUES (?, ?, 0)`, childConflictID, parentID).Error
	assert.Error(t, err)

	// FindAll should return them ordered by sort_order
	cats, err := repo.FindAll(ctx, "uk")
	require.NoError(t, err)
	var parentIndex, root2Index int = -1, -1
	for idx, c := range cats {
		if c.ID == parentID {
			parentIndex = idx
		} else if c.ID == root2ID {
			root2Index = idx
		}
	}
	assert.True(t, parentIndex != -1 && root2Index != -1)
	assert.Less(t, parentIndex, root2Index)
}

func TestCategoryRepository_SortOrderShifting_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewCategoryRepository(gormDB, &noopLogger{})

	// Clean up categories just in case
	gormDB.Exec("DELETE FROM category")

	// 1. Test creation shifting
	// Create root category A with sort_order = 0
	catA := &domain.Category{ID: uuid.New(), SortOrder: 0}
	err := repo.Create(ctx, catA)
	require.NoError(t, err)

	// Create root category B with sort_order = 1
	catB := &domain.Category{ID: uuid.New(), SortOrder: 1}
	err = repo.Create(ctx, catB)
	require.NoError(t, err)

	// Create root category C with sort_order = 1 (should shift B to 2, C takes 1)
	catC := &domain.Category{ID: uuid.New(), SortOrder: 1}
	err = repo.Create(ctx, catC)
	require.NoError(t, err)

	// Fetch all to verify orders: A=0, C=1, B=2
	cats, err := repo.FindAll(ctx, "uk")
	require.NoError(t, err)
	require.Len(t, cats, 3)
	assert.Equal(t, catA.ID, cats[0].ID)
	assert.Equal(t, 0, cats[0].SortOrder)
	assert.Equal(t, catC.ID, cats[1].ID)
	assert.Equal(t, 1, cats[1].SortOrder)
	assert.Equal(t, catB.ID, cats[2].ID)
	assert.Equal(t, 2, cats[2].SortOrder)

	// 2. Test update shifting (within same parent / root level)
	// Currently: A=0, C=1, B=2
	// Move B (2) to 0 (should shift A to 1, C to 2, B takes 0)
	catB.SortOrder = 0
	err = repo.Update(ctx, catB)
	require.NoError(t, err)

	cats, err = repo.FindAll(ctx, "uk")
	require.NoError(t, err)
	require.Len(t, cats, 3)
	assert.Equal(t, catB.ID, cats[0].ID)
	assert.Equal(t, 0, cats[0].SortOrder)
	assert.Equal(t, catA.ID, cats[1].ID)
	assert.Equal(t, 1, cats[1].SortOrder)
	assert.Equal(t, catC.ID, cats[2].ID)
	assert.Equal(t, 2, cats[2].SortOrder)

	// Move B (0) to 2 (should shift A to 0, C to 1, B takes 2)
	catB.SortOrder = 2
	err = repo.Update(ctx, catB)
	require.NoError(t, err)

	cats, err = repo.FindAll(ctx, "uk")
	require.NoError(t, err)
	assert.Equal(t, catA.ID, cats[0].ID)
	assert.Equal(t, 0, cats[0].SortOrder)
	assert.Equal(t, catC.ID, cats[1].ID)
	assert.Equal(t, 1, cats[1].SortOrder)
	assert.Equal(t, catB.ID, cats[2].ID)
	assert.Equal(t, 2, cats[2].SortOrder)

	// 3. Test shifting when changing ParentID (moving between parents)
	// Sibling children under A:
	// Child 1 with sort_order = 0
	child1 := &domain.Category{ID: uuid.New(), ParentID: &catA.ID, SortOrder: 0}
	err = repo.Create(ctx, child1)
	require.NoError(t, err)

	// Child 2 with sort_order = 1
	child2 := &domain.Category{ID: uuid.New(), ParentID: &catA.ID, SortOrder: 1}
	err = repo.Create(ctx, child2)
	require.NoError(t, err)

	// Child 3 with sort_order = 2
	child3 := &domain.Category{ID: uuid.New(), ParentID: &catA.ID, SortOrder: 2}
	err = repo.Create(ctx, child3)
	require.NoError(t, err)

	// Sibling children under C:
	// Child X with sort_order = 0
	childX := &domain.Category{ID: uuid.New(), ParentID: &catC.ID, SortOrder: 0}
	err = repo.Create(ctx, childX)
	require.NoError(t, err)

	// Now move Child 2 (under A, sort_order = 1) to under C with sort_order = 0
	// This should:
	// - Close gap under A: child3 (2) becomes 1.
	// - Open gap under C: childX (0) becomes 1.
	// - child2 gets parent_id = C, sort_order = 0.
	child2.ParentID = &catC.ID
	child2.SortOrder = 0
	err = repo.Update(ctx, child2)
	require.NoError(t, err)

	// Verify child under A: child1=0, child3=1
	var c1, c3 domain.Category
	err = gormDB.Where("id = ?", child1.ID).First(&c1).Error
	require.NoError(t, err)
	assert.Equal(t, 0, c1.SortOrder)

	err = gormDB.Where("id = ?", child3.ID).First(&c3).Error
	require.NoError(t, err)
	assert.Equal(t, 1, c3.SortOrder)

	// Verify child under C: child2=0, childX=1
	var c2, cX domain.Category
	err = gormDB.Where("id = ?", child2.ID).First(&c2).Error
	require.NoError(t, err)
	assert.Equal(t, 0, c2.SortOrder)

	err = gormDB.Where("id = ?", childX.ID).First(&cX).Error
	require.NoError(t, err)
	assert.Equal(t, 1, cX.SortOrder)
}

func TestCategoryRepository_UpdateOrder_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	gormDB, cleanup := db.SetupTestDB(t)
	defer cleanup()

	repo := NewCategoryRepository(gormDB, &noopLogger{})

	// Clean up categories just in case
	gormDB.Exec("DELETE FROM category")

	// Seed 3 categories
	catA := &domain.Category{ID: uuid.New(), SortOrder: 0}
	require.NoError(t, repo.Create(ctx, catA))

	catB := &domain.Category{ID: uuid.New(), SortOrder: 1}
	require.NoError(t, repo.Create(ctx, catB))

	catC := &domain.Category{ID: uuid.New(), SortOrder: 2}
	require.NoError(t, repo.Create(ctx, catC))

	// Reorder them: catC (becomes 0), catA (becomes 1), catB (becomes 2)
	err := repo.UpdateOrder(ctx, []uuid.UUID{catC.ID, catA.ID, catB.ID})
	require.NoError(t, err)

	// Fetch all to verify order
	cats, err := repo.FindAll(ctx, "uk")
	require.NoError(t, err)
	require.Len(t, cats, 3)

	assert.Equal(t, catC.ID, cats[0].ID)
	assert.Equal(t, 0, cats[0].SortOrder)

	assert.Equal(t, catA.ID, cats[1].ID)
	assert.Equal(t, 1, cats[1].SortOrder)

	assert.Equal(t, catB.ID, cats[2].ID)
	assert.Equal(t, 2, cats[2].SortOrder)
}

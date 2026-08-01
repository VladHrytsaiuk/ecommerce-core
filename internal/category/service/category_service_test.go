package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
)

func newCatSvc(repo *MockCategoryRepository, pc *MockProductChecker) domain.CategoryService {
	return NewCategoryService(repo, pc, nil, nil, &noopLogger{})
}



func makeCategoryWithTranslation(lang, name string) domain.Category {
	return domain.Category{
		ID:       uuid.New(),
		ParentID: nil,
		Translations: []domain.CategoryTranslation{
			{LanguageCode: lang, Name: name},
		},
	}
}

// ==========================================
// GetList
// ==========================================

func TestCategoryService_GetList_HappyPath(t *testing.T) {
	repo := &MockCategoryRepository{}
	pc := &MockProductChecker{}
	svc := newCatSvc(repo, pc)

	categories := []domain.Category{
		makeCategoryWithTranslation("uk", "Шампуні"),
		makeCategoryWithTranslation("uk", "Кондиціонери"),
	}
	repo.On("FindAll", mock.Anything, "uk").Return(categories, nil)

	result, err := svc.GetList(context.Background(), "uk", true)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "Шампуні", result[0].Translations[0].Name)
	assert.Equal(t, "Кондиціонери", result[1].Translations[0].Name)
}

func TestCategoryService_GetList_EmptyResult(t *testing.T) {
	repo := &MockCategoryRepository{}
	pc := &MockProductChecker{}
	svc := newCatSvc(repo, pc)

	repo.On("FindAll", mock.Anything, "en").Return([]domain.Category{}, nil)

	result, err := svc.GetList(context.Background(), "en", true)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestCategoryService_GetList_RepoError(t *testing.T) {
	repo := &MockCategoryRepository{}
	pc := &MockProductChecker{}
	svc := newCatSvc(repo, pc)

	repo.On("FindAll", mock.Anything, "uk").Return(nil, fmt.Errorf("db connection failed"))

	result, err := svc.GetList(context.Background(), "uk", true)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db connection failed")
}

func TestCategoryService_GetList_WithParentID(t *testing.T) {
	repo := &MockCategoryRepository{}
	pc := &MockProductChecker{}
	svc := newCatSvc(repo, pc)

	parentID := uuid.New()
	subcategory := domain.Category{
		ID:       uuid.New(),
		ParentID: &parentID,
		Translations: []domain.CategoryTranslation{
			{LanguageCode: "uk", Name: "Підкатегорія"},
		},
	}

	repo.On("FindAll", mock.Anything, "uk").Return([]domain.Category{subcategory}, nil)

	result, err := svc.GetList(context.Background(), "uk", true)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.NotNil(t, result[0].ParentID)
	assert.Equal(t, parentID, *result[0].ParentID)
}

func TestCategoryService_GetList_PassesLanguageToRepo(t *testing.T) {
	repo := &MockCategoryRepository{}
	pc := &MockProductChecker{}
	svc := newCatSvc(repo, pc)

	repo.On("FindAll", mock.Anything, "en").Return([]domain.Category{}, nil)

	_, _ = svc.GetList(context.Background(), "en", true)

	repo.AssertCalled(t, "FindAll", mock.Anything, "en")
}

func TestCategoryService_GetList_HierarchicalSorting(t *testing.T) {
	repo := &MockCategoryRepository{}
	pc := &MockProductChecker{}
	svc := newCatSvc(repo, pc)

	rootAID := uuid.New()
	rootBID := uuid.New()
	childA1ID := uuid.New()
	childA2ID := uuid.New()
	childB1ID := uuid.New()

	// Mix them up but sorted by sort_order, simulating database retrieval where sorting by sort_order alone is not hierarchical
	categories := []domain.Category{
		{ID: rootAID, ParentID: nil, SortOrder: 0},
		{ID: childA1ID, ParentID: &rootAID, SortOrder: 0},
		{ID: childB1ID, ParentID: &rootBID, SortOrder: 0},
		{ID: rootBID, ParentID: nil, SortOrder: 1},
		{ID: childA2ID, ParentID: &rootAID, SortOrder: 1},
	}

	repo.On("FindAll", mock.Anything, "uk").Return(categories, nil)

	result, err := svc.GetList(context.Background(), "uk", true)
	require.NoError(t, err)
	require.Len(t, result, 5)

	// Expected hierarchical order:
	// 1. Root A (0)
	// 2. Child A.1 (0)
	// 3. Child A.2 (1)
	// 4. Root B (1)
	// 5. Child B.1 (0)
	assert.Equal(t, rootAID, result[0].ID)
	assert.Equal(t, childA1ID, result[1].ID)
	assert.Equal(t, childA2ID, result[2].ID)
	assert.Equal(t, rootBID, result[3].ID)
	assert.Equal(t, childB1ID, result[4].ID)
}

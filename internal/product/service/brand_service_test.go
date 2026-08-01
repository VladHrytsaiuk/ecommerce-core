package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func newBrandSvc(repo *MockBrandRepository, pr *MockProductRepository) domain.BrandService {
	return NewBrandService(repo, pr, &noopLogger{})
}

// ==========================================
// GetAll
// ==========================================

func TestBrandService_GetAll_HappyPath(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	brands := []domain.Brand{
		{ID: uuid.New(), Name: "Brand A"},
		{ID: uuid.New(), Name: "Brand B"},
	}

	repo.On("FindAll", mock.Anything).Return(brands, nil)

	result, err := svc.GetAll(context.Background())

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "Brand A", result[0].Name)
	assert.Equal(t, "Brand B", result[1].Name)
}

func TestBrandService_GetAll_EmptyList(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	repo.On("FindAll", mock.Anything).Return([]domain.Brand{}, nil)

	result, err := svc.GetAll(context.Background())

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestBrandService_GetAll_RepoError(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	repo.On("FindAll", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	result, err := svc.GetAll(context.Background())

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// ==========================================
// GetByID
// ==========================================

func TestBrandService_GetByID(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)
	
	id := uuid.New()
	expected := &domain.Brand{ID: id, Name: "Brand A"}
	repo.On("FindByID", mock.Anything, id).Return(expected, nil)

	res, err := svc.GetByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, expected, res)
}

// ==========================================
// GenerateSlug
// ==========================================

func TestBrandService_GenerateSlug(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	repo.On("SlugExists", mock.Anything, "my-brand").Return(true, nil).Once()
	repo.On("SlugExists", mock.Anything, "my-brand-1").Return(false, nil).Once()

	slug, err := svc.GenerateSlug(context.Background(), "My Brand!")
	require.NoError(t, err)
	assert.Equal(t, "my-brand-1", slug)
}

// ==========================================
// Create
// ==========================================

func TestBrandService_Create(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	brand := &domain.Brand{Name: "New Brand"}

	repo.On("SlugExists", mock.Anything, "new-brand").Return(false, nil)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Brand")).Return(nil)

	err := svc.Create(context.Background(), brand)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, brand.ID)
	assert.Equal(t, "new-brand", brand.Slug)
}

// ==========================================
// Update
// ==========================================

func TestBrandService_Update(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	brand := &domain.Brand{ID: uuid.New(), Name: "Updated Brand", Slug: "updated-brand"}

	repo.On("Update", mock.Anything, brand).Return(nil)

	err := svc.Update(context.Background(), brand)
	require.NoError(t, err)
}

// ==========================================
// Delete
// ==========================================

func TestBrandService_Delete(t *testing.T) {
	repo := &MockBrandRepository{}
	pr := &MockProductRepository{}
	svc := newBrandSvc(repo, pr)

	id := uuid.New()

	// Case 1: Brand in use
	pr.On("CountByBrand", mock.Anything, id).Return(1, nil).Once()
	err := svc.Delete(context.Background(), id)
	assert.ErrorIs(t, err, domain.ErrBrandInUse)

	// Case 2: Success
	pr.On("CountByBrand", mock.Anything, id).Return(0, nil).Once()
	repo.On("Delete", mock.Anything, id).Return(nil).Once()
	err = svc.Delete(context.Background(), id)
	require.NoError(t, err)
}

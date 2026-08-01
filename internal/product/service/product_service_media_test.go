package service

import (
	"context"
	"mime/multipart"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func TestProductService_DeleteProduct(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)
	ctx := context.Background()
	id := uuid.New()

	repo.On("Exists", ctx, id).Return(true, nil)
	repo.On("Delete", ctx, id).Return(nil)

	err := svc.DeleteProduct(ctx, id)
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestProductService_DeleteProductImage(t *testing.T) {
	repo := &MockProductRepository{}
	storage := &MockStorage{}
	svc := NewProductService(repo, storage, nil, nil, &noopLogger{})
	ctx := context.Background()

	productID := uuid.New()
	imgID := uuid.New()
	img := &domain.ProductImage{
		ID: imgID,
		ProductID: productID,
		ImageURL: "http://storage/products/img.jpg",
	}

	repo.On("FindImageByID", ctx, imgID).Return(img, nil)
	storage.On("Delete", mock.Anything, "products/img").Return(nil)
	repo.On("DeleteImage", ctx, imgID).Return(nil)

	err := svc.DeleteProductImage(ctx, productID, imgID)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	
	repo.AssertExpectations(t)
	storage.AssertExpectations(t)
}

func TestProductService_UpdateProductImage(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)
	ctx := context.Background()

	productID := uuid.New()
	imgID := uuid.New()
	img := &domain.ProductImage{
		ID: imgID,
		ProductID: productID,
		ImageURL: "http://storage/products/img2.jpg",
		IsPrimary: true,
		VariationID: nil,
	}

	isPrimary := true
	
	repo.On("FindImageByID", ctx, imgID).Return(img, nil)
	repo.On("ResetImageRole", ctx, productID, (*uuid.UUID)(nil), "is_primary").Return(nil)
	repo.On("UpdateImage", ctx, img).Return(nil)

	err := svc.UpdateProductImage(ctx, productID, imgID, &isPrimary, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestProductService_ReorderProductImages(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)
	ctx := context.Background()

	productID := uuid.New()
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	repo.On("ReorderImages", ctx, productID, ids).Return(nil)

	err := svc.ReorderProductImages(ctx, productID, ids)
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestProductService_UploadMedia(t *testing.T) {
	storage := &MockStorage{}
	svc := NewProductService(nil, storage, nil, nil, &noopLogger{})
	ctx := context.Background()

	file := &multipart.FileHeader{Filename: "test.jpg"}

	storage.On("Upload", ctx, file, "media", "test.jpg").Return("http://storage/test.jpg", nil)

	url, err := svc.UploadMedia(ctx, file, "media", "test.jpg")
	require.NoError(t, err)
	assert.Equal(t, "http://storage/test.jpg", url)
	storage.AssertExpectations(t)
}

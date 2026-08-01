package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

func newProdSvc(repo *MockProductRepository) domain.ProductService {
	return NewProductService(repo, &MockStorage{}, nil, &config.Config{}, &noopLogger{})
}

// ==========================================
// GetList
// ==========================================

func TestProductService_GetList_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	filter := domain.ProductFilter{}
	pgn := pagination.Params{Page: 1, Limit: 20, SortBy: "created_at", Order: "desc"}

	variations := []domain.ProductVariation{
		{ID: uuid.New(), SKU: "SKU-001", Price: 10000},
		{ID: uuid.New(), SKU: "SKU-002", Price: 20000},
	}

	repo.On("FindAll", mock.Anything, "uk", filter, pgn).Return(variations, int64(2), nil)

	result, meta, err := svc.GetList(context.Background(), "uk", filter, pgn)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, int64(2), meta.TotalItems)
	assert.Equal(t, 1, meta.CurrentPage)
	assert.Equal(t, 20, meta.PageSize)
}

func TestProductService_GetList_EmptyResult(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	filter := domain.ProductFilter{}
	pgn := pagination.Params{Page: 1, Limit: 20, SortBy: "created_at", Order: "desc"}

	repo.On("FindAll", mock.Anything, "uk", filter, pgn).Return([]domain.ProductVariation{}, int64(0), nil)

	result, meta, err := svc.GetList(context.Background(), "uk", filter, pgn)

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, int64(0), meta.TotalItems)
}

func TestProductService_GetList_RepoError(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	filter := domain.ProductFilter{}
	pgn := pagination.Params{Page: 1, Limit: 20, SortBy: "created_at", Order: "desc"}

	repo.On("FindAll", mock.Anything, "uk", filter, pgn).Return(nil, int64(0), fmt.Errorf("db error"))

	result, meta, err := svc.GetList(context.Background(), "uk", filter, pgn)

	assert.Nil(t, result)
	assert.Equal(t, pagination.Metadata{}, meta)
	assert.Error(t, err)
}

func TestProductService_GetList_PaginationMetadata(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	filter := domain.ProductFilter{}
	pgn := pagination.Params{Page: 2, Limit: 10, SortBy: "price", Order: "asc"}

	variations := make([]domain.ProductVariation, 10)
	for i := range variations {
		variations[i] = domain.ProductVariation{ID: uuid.New(), Price: i * 1000}
	}

	repo.On("FindAll", mock.Anything, "uk", filter, pgn).Return(variations, int64(25), nil)

	_, meta, err := svc.GetList(context.Background(), "uk", filter, pgn)

	require.NoError(t, err)
	assert.Equal(t, 2, meta.CurrentPage)
	assert.Equal(t, 10, meta.PageSize)
	assert.Equal(t, int64(25), meta.TotalItems)
	assert.Equal(t, 3, meta.TotalPages) // ceil(25/10) = 3
	assert.True(t, meta.HasNextPage)    // page 2 < 3
	assert.True(t, meta.HasPrevPage)    // page 2 > 1
}

// ==========================================
// GetByID
// ==========================================

func TestProductService_GetByID_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()
	expected := &domain.Product{
		ID:       productID,
		IsActive: true,
		Translations: []domain.ProductTranslation{
			{ProductID: productID, LanguageCode: "uk", Name: "Тестовий товар"},
		},
	}

	repo.On("FindByID", mock.Anything, productID, "uk").Return(expected, nil)

	result, err := svc.GetByID(context.Background(), productID, "uk")

	require.NoError(t, err)
	assert.Equal(t, productID, result.ID)
}

func TestProductService_GetByID_NotFound(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()
	repo.On("FindByID", mock.Anything, productID, "uk").Return(nil, domain.ErrProductNotFound)

	result, err := svc.GetByID(context.Background(), productID, "uk")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)
}

// ==========================================
// GetBySlug
// ==========================================

func TestProductService_GetBySlug_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	expected := &domain.Product{
		ID:   uuid.New(),

		Translations: []domain.ProductTranslation{
			{LanguageCode: "uk", Name: "Набір капсул для кави", Slug: "nabir-kapsul-dlia-kavy"},
		},
	}

	repo.On("FindBySlug", mock.Anything, "nabir-kapsul-dlia-kavy", "uk").Return(expected, nil)

	result, err := svc.GetBySlug(context.Background(), "nabir-kapsul-dlia-kavy", "uk")

	require.NoError(t, err)
	assert.Equal(t, "nabir-kapsul-dlia-kavy", result.Translations[0].Slug)
}

func TestProductService_GetBySlug_NotFound(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	repo.On("FindBySlug", mock.Anything, "nonexistent", "uk").Return(nil, domain.ErrSlugNotFound)

	result, err := svc.GetBySlug(context.Background(), "nonexistent", "uk")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrSlugNotFound)
}

// ==========================================
// GenerateSlug
// ==========================================

func TestProductService_GenerateSlug_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(false, nil).Once()

	slug, err := svc.GenerateSlug(context.Background(), "Набір капсул для кави", uuid.Nil)

	require.NoError(t, err)
	assert.NotEmpty(t, slug)
	assert.NotContains(t, slug, " ")
}

func TestProductService_GenerateSlug_CollisionHandled(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	// Перший виклик — slug вже існує, другий — з суфіксом — вільний
	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(true, nil).Once()
	repo.On("SlugExists", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(false, nil).Once()

	slug, err := svc.GenerateSlug(context.Background(), "Тестовий товар", uuid.Nil)

	require.NoError(t, err)
	assert.NotEmpty(t, slug)
	assert.Contains(t, slug, "-") // Має містити суфікс через колізію
}

func TestProductService_GenerateSlug_EmptyName(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	repo.On("SlugExists", mock.Anything, "product", mock.Anything).Return(false, nil).Once()

	slug, err := svc.GenerateSlug(context.Background(), "", uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, "product", slug)
}

func TestProductService_GenerateSlug_RepoError(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	repo.On("SlugExists", mock.Anything, mock.Anything, mock.Anything).Return(false, fmt.Errorf("db error"))

	slug, err := svc.GenerateSlug(context.Background(), "Товар", uuid.Nil)

	assert.Empty(t, slug)
	assert.Error(t, err)
}

// ==========================================
// GetReviews
// ==========================================

func TestProductService_GetReviews_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()
	pgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	reviews := []domain.ProductReview{
		{ID: uuid.New(), ProductID: productID, Rating: 5, Comment: "Чудово!", Status: "approved"},
		{ID: uuid.New(), ProductID: productID, Rating: 4, Comment: "Добре", Status: "approved"},
	}

	repo.On("Exists", mock.Anything, productID).Return(true, nil)
	repo.On("FindReviews", mock.Anything, productID, pgn).Return(reviews, int64(2), nil)

	result, meta, err := svc.GetReviews(context.Background(), productID, pgn)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, int64(2), meta.TotalItems)
}

func TestProductService_GetReviews_ProductNotFound(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()
	pgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	repo.On("Exists", mock.Anything, productID).Return(false, nil)

	result, _, err := svc.GetReviews(context.Background(), productID, pgn)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestProductService_GetReviews_ExistsCheckFails(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()
	pgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	repo.On("Exists", mock.Anything, productID).Return(false, fmt.Errorf("db error"))

	_, _, err := svc.GetReviews(context.Background(), productID, pgn)

	assert.Error(t, err)
}

func TestProductService_GetReviews_EmptyReviews(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	productID := uuid.New()
	pgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	repo.On("Exists", mock.Anything, productID).Return(true, nil)
	repo.On("FindReviews", mock.Anything, productID, pgn).Return([]domain.ProductReview{}, int64(0), nil)

	result, meta, err := svc.GetReviews(context.Background(), productID, pgn)

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, int64(0), meta.TotalItems)
}

// ==========================================
// GetPendingReviews
// ==========================================

func TestProductService_GetPendingReviews_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	pgn := pagination.Params{Page: 1, Limit: 10, SortBy: "created_at", Order: "desc"}

	reviews := []domain.ProductReview{
		{ID: uuid.New(), ProductID: uuid.New(), Rating: 5, Comment: "Pending 1", Status: "pending"},
	}

	repo.On("FindPendingReviews", mock.Anything, pgn).Return(reviews, int64(1), nil)

	result, meta, err := svc.GetPendingReviews(context.Background(), pgn)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, int64(1), meta.TotalItems)
}

// ==========================================
// AddReview
// ==========================================

func TestProductService_AddReview_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	review := &domain.ProductReview{
		ProductID:  uuid.New(),
		UserID:     uuid.New(),
		Rating:     5,
		Comment:    "Відмінний товар!",
		Status: "pending",
	}

	repo.On("CreateReview", mock.Anything, mock.MatchedBy(func(r *domain.ProductReview) bool {
		return r.Rating == 5 && r.ID != uuid.Nil
	})).Return(nil)

	err := svc.AddReview(context.Background(), review)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, review.ID, "ID повинен бути згенерований")
}

func TestProductService_AddReview_InvalidRating_TooLow(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	review := &domain.ProductReview{
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    0,
		Comment:   "Погано",
	}

	err := svc.AddReview(context.Background(), review)

	assert.ErrorIs(t, err, domain.ErrInvalidRating)
}

func TestProductService_AddReview_InvalidRating_TooHigh(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	review := &domain.ProductReview{
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    6,
		Comment:   "Супер!",
	}

	err := svc.AddReview(context.Background(), review)

	assert.ErrorIs(t, err, domain.ErrInvalidRating)
}

func TestProductService_AddReview_CommentTooLong(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	longComment := make([]byte, 2001)
	for i := range longComment {
		longComment[i] = 'a'
	}

	review := &domain.ProductReview{
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    3,
		Comment:   string(longComment),
	}

	err := svc.AddReview(context.Background(), review)

	assert.ErrorIs(t, err, domain.ErrCommentTooLong)
}

// Сервіс більше не перевизначає Status — хендлер сам керує цим значенням
func TestProductService_AddReview_StatusPassedThrough(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	review := &domain.ProductReview{
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    5,
		Comment:   "Good",
		Status:    "approved", // адмін може передати approved — сервіс не перевизначає
	}

	repo.On("CreateReview", mock.Anything, mock.MatchedBy(func(r *domain.ProductReview) bool {
		return r.Status == "approved"
	})).Return(nil)

	err := svc.AddReview(context.Background(), review)

	require.NoError(t, err)
	assert.Equal(t, "approved", review.Status, "сервіс не повинен перевизначати Status")
	repo.AssertExpectations(t)
}

func TestProductService_AddReview_Reply_SkipsRatingValidation(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	parentID := uuid.New()
	review := &domain.ProductReview{
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    0, // не валідний для звичайного відгуку, але дозволений для reply
		Comment:   "Дякуємо за відгук!",
		ParentID:  &parentID,
	}

	repo.On("CreateReview", mock.Anything, mock.MatchedBy(func(r *domain.ProductReview) bool {
		return r.Rating == 0 && r.ParentID != nil
	})).Return(nil)

	err := svc.AddReview(context.Background(), review)

	require.NoError(t, err, "reply з rating=0 не повинен повертати помилку")
	assert.Equal(t, 0, review.Rating, "rating для reply має залишатися 0")
}

func TestProductService_AddReview_GeneratesID(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	review := &domain.ProductReview{
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    3,
		Comment:   "Нормально",
	}

	repo.On("CreateReview", mock.Anything, mock.Anything).Return(nil)

	err := svc.AddReview(context.Background(), review)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, review.ID)
}

func TestProductService_AddReview_KeepsExistingID(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	existingID := uuid.New()
	review := &domain.ProductReview{
		ID:        existingID,
		ProductID: uuid.New(),
		UserID:    uuid.New(),
		Rating:    4,
		Comment:   "Добре",
	}

	repo.On("CreateReview", mock.Anything, mock.Anything).Return(nil)

	err := svc.AddReview(context.Background(), review)

	require.NoError(t, err)
	assert.Equal(t, existingID, review.ID, "існуючий ID не повинен бути перезаписаний")
}

// ==========================================
// ApproveReview
// ==========================================

func TestProductService_ApproveReview_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	reviewID := uuid.New()
	repo.On("ApproveReview", mock.Anything, reviewID).Return(nil)

	err := svc.ApproveReview(context.Background(), reviewID)

	require.NoError(t, err)
}

func TestProductService_ApproveReview_NotFound(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	reviewID := uuid.New()
	repo.On("ApproveReview", mock.Anything, reviewID).Return(domain.ErrReviewNotFound)

	err := svc.ApproveReview(context.Background(), reviewID)

	assert.ErrorIs(t, err, domain.ErrReviewNotFound)
}

// ==========================================
// GetFilters
// ==========================================

func TestProductService_GetFilters_HappyPath(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	filter := domain.ProductFilter{CategoryID: []uuid.UUID{uuid.New()}}
	expected := &domain.FilterDiscovery{
		MinPrice: 1000,
		MaxPrice: 50000,
		Brands: []domain.BrandFilterOption{
			{ID: uuid.New(), Name: "Brand A", Count: 10},
		},
	}

	repo.On("GetFilters", mock.Anything, filter, "uk").Return(expected, nil)

	result, err := svc.GetFilters(context.Background(), filter, "uk")

	require.NoError(t, err)
	assert.Equal(t, 1000, result.MinPrice)
	assert.Equal(t, 50000, result.MaxPrice)
	assert.Len(t, result.Brands, 1)
}

func TestProductService_GetFilters_RepoError(t *testing.T) {
	repo := &MockProductRepository{}
	svc := newProdSvc(repo)

	filter := domain.ProductFilter{}
	repo.On("GetFilters", mock.Anything, filter, "uk").Return(nil, fmt.Errorf("db error"))

	result, err := svc.GetFilters(context.Background(), filter, "uk")

	assert.Nil(t, result)
	assert.Error(t, err)
}

// ==========================================
// Badge Computation Tests
// ==========================================

func TestProductService_ComputeProductBadges(t *testing.T) {
	repo := &MockProductRepository{}
	cfg := &config.Config{
		BadgeNewDays:             7,
		BadgeBestSellerThreshold: 5,
	}
	svc := NewProductService(repo, &MockStorage{}, nil, cfg, &noopLogger{}).(*productService)

	t.Run("New Product Badge", func(t *testing.T) {
		p := &domain.Product{
			CreatedAt: time.Now().Add(-1 * time.Hour), // Very new
		}
		svc.computeProductBadges(p)
		require.Len(t, p.ComputedBadges, 1)
		assert.Equal(t, BadgeIDNew, p.ComputedBadges[0].ID)
		assert.Equal(t, "Новинка", p.ComputedBadges[0].Name["uk"])
	})

	t.Run("Best Seller Badge", func(t *testing.T) {
		p := &domain.Product{
			CreatedAt:    time.Now().Add(-30 * 24 * time.Hour), // Old
			ReviewsCount: 10,                                   // Meets threshold 5
		}
		svc.computeProductBadges(p)
		require.Len(t, p.ComputedBadges, 1)
		assert.Equal(t, BadgeIDBestSeller, p.ComputedBadges[0].ID)
		assert.Equal(t, "Хіт продажів", p.ComputedBadges[0].Name["uk"])
	})

	t.Run("Both Badges", func(t *testing.T) {
		p := &domain.Product{
			CreatedAt:    time.Now().Add(-1 * time.Hour), // New
			ReviewsCount: 10,                             // Best seller
		}
		svc.computeProductBadges(p)
		require.Len(t, p.ComputedBadges, 2)
	})

	t.Run("No Badges", func(t *testing.T) {
		p := &domain.Product{
			CreatedAt:    time.Now().Add(-30 * 24 * time.Hour), // Old
			ReviewsCount: 2,                                    // Below threshold
		}
		svc.computeProductBadges(p)
		require.Len(t, p.ComputedBadges, 0)
	})

	t.Run("Nil Product", func(t *testing.T) {
		assert.NotPanics(t, func() {
			svc.computeProductBadges(nil)
		})
	})
}

func TestProductService_ComputeVariationBadges(t *testing.T) {
	repo := &MockProductRepository{}
	svc := NewProductService(repo, &MockStorage{}, nil, &config.Config{}, &noopLogger{}).(*productService)

	t.Run("Sale Badge Computed", func(t *testing.T) {
		oldPrice := 10000
		v := &domain.ProductVariation{
			Price:    8000,
			OldPrice: &oldPrice,
		}
		svc.computeVariationBadges(v)
		require.Len(t, v.ComputedBadges, 1)
		assert.Equal(t, BadgeIDSale, v.ComputedBadges[0].ID)
		// Discount = (10000 - 8000)*100 / 10000 = 20%
		assert.Equal(t, "Знижка 20%", v.ComputedBadges[0].Name["uk"])
		assert.Equal(t, "Sale 20%", v.ComputedBadges[0].Name["en"])
	})

	t.Run("No Sale Badge if OldPrice is nil", func(t *testing.T) {
		v := &domain.ProductVariation{
			Price: 8000,
		}
		svc.computeVariationBadges(v)
		require.Len(t, v.ComputedBadges, 0)
	})

	t.Run("No Sale Badge if OldPrice <= Price", func(t *testing.T) {
		oldPrice := 8000
		v := &domain.ProductVariation{
			Price:    8000,
			OldPrice: &oldPrice,
		}
		svc.computeVariationBadges(v)
		require.Len(t, v.ComputedBadges, 0)
	})

	t.Run("Nil Variation", func(t *testing.T) {
		assert.NotPanics(t, func() {
			svc.computeVariationBadges(nil)
		})
	})
}

package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// localeMW імітує LocaleMiddleware для тестів
func localeMW() gin.HandlerFunc {
	return func(c *gin.Context) {
		l := c.Param("lang")
		if l == "ua" {
			l = "uk"
		}
		if l != "uk" && l != "en" {
			l = "uk"
		}
		c.Set("language", l)
		c.Next()
	}
}

// newProductRouter створює тестовий роутер для публічних GET ендпоінтів
func newProductRouter(svc *MockProductService) *gin.Engine {
	r := gin.New()
	h := NewProductHandler(svc, nil, nil, &noopLogger{})

	langGroup := r.Group("/api/:lang")
	langGroup.Use(localeMW())
	langGroup.GET("/products", h.GetProducts)
	langGroup.GET("/products/filters", h.GetProductFilters)
	langGroup.GET("/products/:id", h.GetProductByID)

	r.GET("/api/products/:id/reviews", h.GetProductReviews)
	return r
}

// newProductAuthRouter створює роутер з auth контекстом для POST ендпоінтів
func newProductAuthRouter(svc *MockProductService, userID uuid.UUID) *gin.Engine {
	r := gin.New()
	h := NewProductHandler(svc, nil, nil, &noopLogger{})

	inject := func(c *gin.Context) {
		c.Set("user_id", userID.String())
		c.Next()
	}

	r.POST("/api/products/:id/reviews", inject, h.CreateReview)
	return r
}

// newProductAdminRouter — роутер для admin-only ендпоінтів
func newProductAdminRouter(svc *MockProductService) *gin.Engine {
	r := gin.New()
	h := NewProductHandler(svc, nil, nil, &noopLogger{})

	r.PATCH("/api/admin/reviews/:id/approve", h.ApproveReview)
	r.PATCH("/api/admin/reviews/:id/reject", h.RejectReview)
	r.GET("/api/admin/reviews/pending", h.GetPendingReviews)
	return r
}

// makeTestProduct створює тестовий Product з перекладами
func makeTestProduct(id uuid.UUID, lang, name string) *domain.Product {
	brandID := uuid.New()
	return &domain.Product{
		ID:         id,
		BrandID:    brandID,
		CategoryID: uuid.New(),
		IsActive:   true,
		Brand:      domain.Brand{ID: brandID, Name: "Test Brand"},
		Translations: []domain.ProductTranslation{
			{ProductID: id, LanguageCode: lang, Name: name, Description: "Опис товару"},
		},
		Variations: []domain.ProductVariation{
			{ID: uuid.New(), ProductID: id, SKU: "SKU-001", Price: 10000, IsActive: true},
		},
		Images: []domain.ProductImage{},
	}
}

// ==========================================
// GetProducts
// ==========================================

func TestProductHandler_GetProducts_Success(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	prodID := uuid.New()
	variations := []domain.ProductVariation{
		{
			ID:        uuid.New(),
			ProductID: prodID,
			SKU:       "SKU-001",
			Price:     15000,
			Product: domain.Product{
				ID:           prodID,
				Brand:        domain.Brand{Name: "Brand"},
				Translations: []domain.ProductTranslation{{LanguageCode: "uk", Name: "Шампунь"}},
				Images:       []domain.ProductImage{},
			},
		},
	}
	meta := pagination.Metadata{
		CurrentPage: 1,
		PageSize:    20,
		TotalItems:  1,
		TotalPages:  1,
	}

	svc.On("GenerateSlug", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return("test-slug", nil)

	svc.On("GetList", mock.Anything, "uk", mock.Anything, mock.Anything).Return(variations, meta, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp pagination.PagedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Metadata.CurrentPage)
}

func TestProductHandler_GetProducts_DynamicAttributes(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	meta := pagination.Metadata{CurrentPage: 1, PageSize: 20, TotalItems: 0, TotalPages: 0}

	svc.On("GetList", mock.Anything, "uk", mock.MatchedBy(func(filter domain.ProductFilter) bool {
		return len(filter.AttrValues["type"]) == 2 &&
			filter.AttrValues["type"][0] == "capsules" &&
			filter.AttrValues["type"][1] == "tablets"
	}), mock.Anything).Return([]domain.ProductVariation{}, meta, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products?attrs[type]=capsules,tablets", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
}

func TestProductHandler_GetProducts_DynamicAttributes_Swagger(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	meta := pagination.Metadata{CurrentPage: 1, PageSize: 20, TotalItems: 0, TotalPages: 0}

	svc.On("GetList", mock.Anything, "uk", mock.MatchedBy(func(filter domain.ProductFilter) bool {
		return len(filter.AttrValues["type"]) == 2 &&
			filter.AttrValues["type"][0] == "capsules" &&
			filter.AttrValues["type"][1] == "tablets"
	}), mock.Anything).Return([]domain.ProductVariation{}, meta, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products?attrs[code]=type=capsules,tablets", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
}

func TestProductHandler_GetProducts_WithPagination(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	meta := pagination.Metadata{CurrentPage: 2, PageSize: 10, TotalItems: 25, TotalPages: 3, HasNextPage: true, HasPrevPage: true}
	svc.On("GetList", mock.Anything, "uk", mock.Anything, mock.Anything).Return([]domain.ProductVariation{}, meta, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products?page=2&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProductHandler_GetProducts_ServiceError(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	svc.On("GetList", mock.Anything, "uk", mock.Anything, mock.Anything).Return(nil, pagination.Metadata{}, fmt.Errorf("service error"))

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestProductHandler_GetProducts_InvalidPagination(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products?page=-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// GetProductByID
// ==========================================

func TestProductHandler_GetProductByID_Success(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	productID := uuid.New()
	product := makeTestProduct(productID, "uk", "Тестовий товар")

	svc.On("GetByID", mock.Anything, productID, "uk").Return(product, nil)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/uk/products/%s", productID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ProductResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, productID, resp.ID)
	assert.Equal(t, "Тестовий товар", resp.Name)
}

func TestProductHandler_GetProductByID_NotFound(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	productID := uuid.New()
	svc.On("GetByID", mock.Anything, productID, "uk").Return(nil, domain.ErrProductNotFound)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/uk/products/%s", productID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProductHandler_GetProductByID_InvalidUUID(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// GetProductReviews
// ==========================================

func TestProductHandler_GetProductReviews_Success(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	productID := uuid.New()
	reviews := []domain.ProductReview{
		{ID: uuid.New(), ProductID: productID, Rating: 5, Comment: "Чудово!", Status: "approved"},
	}
	meta := pagination.Metadata{CurrentPage: 1, PageSize: 10, TotalItems: 1, TotalPages: 1}

	svc.On("GetReviews", mock.Anything, productID, mock.Anything).Return(reviews, meta, nil)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/products/%s/reviews", productID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProductHandler_GetProductReviews_ProductNotFound(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	productID := uuid.New()
	svc.On("GetReviews", mock.Anything, productID, mock.Anything).Return(nil, pagination.Metadata{}, domain.ErrProductNotFound)

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/products/%s/reviews", productID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProductHandler_GetProductReviews_InvalidUUID(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/api/products/bad-uuid/reviews", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// CreateReview
// ==========================================

func TestProductHandler_CreateReview_Success(t *testing.T) {
	svc := &MockProductService{}
	userID := uuid.New()
	router := newProductAuthRouter(svc, userID)

	productID := uuid.New()
	svc.On("AddReview", mock.Anything, mock.MatchedBy(func(r *domain.ProductReview) bool {
		return r.ProductID == productID && r.Rating == 5 && r.UserID == userID
	})).Return(nil)

	body := toJSON(t, map[string]interface{}{
		"rating":  5,
		"comment": "Чудовий товар!",
	})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/reviews", productID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestProductHandler_CreateReview_InvalidRating(t *testing.T) {
	svc := &MockProductService{}
	userID := uuid.New()
	router := newProductAuthRouter(svc, userID)

	productID := uuid.New()
	svc.On("AddReview", mock.Anything, mock.Anything).Return(domain.ErrInvalidRating)

	body := toJSON(t, map[string]interface{}{
		"rating":  6,
		"comment": "Bad rating",
	})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/reviews", productID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProductHandler_CreateReview_NoAuth(t *testing.T) {
	svc := &MockProductService{}
	r := gin.New()
	h := NewProductHandler(svc, nil, nil, &noopLogger{})
	r.POST("/api/products/:id/reviews", h.CreateReview)

	productID := uuid.New()
	body := toJSON(t, map[string]interface{}{
		"rating":  5,
		"comment": "test",
	})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/reviews", productID), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProductHandler_CreateReview_InvalidUUID(t *testing.T) {
	svc := &MockProductService{}
	userID := uuid.New()
	router := newProductAuthRouter(svc, userID)

	body := toJSON(t, map[string]interface{}{
		"rating":  5,
		"comment": "test",
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/products/not-uuid/reviews", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProductHandler_CreateReview_InvalidJSON(t *testing.T) {
	svc := &MockProductService{}
	userID := uuid.New()
	router := newProductAuthRouter(svc, userID)

	productID := uuid.New()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/reviews", productID), nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// ApproveReview
// ==========================================

func TestProductHandler_ApproveReview_Success(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	reviewID := uuid.New()
	svc.On("ApproveReview", mock.Anything, reviewID).Return(nil)

	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/reviews/%s/approve", reviewID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProductHandler_ApproveReview_NotFound(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	reviewID := uuid.New()
	svc.On("ApproveReview", mock.Anything, reviewID).Return(domain.ErrReviewNotFound)

	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/api/admin/reviews/%s/approve", reviewID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProductHandler_ApproveReview_InvalidUUID(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	req, _ := http.NewRequest(http.MethodPatch, "/api/admin/reviews/not-uuid/approve", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProductHandler_GetPendingReviews_Success(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	reviews := []domain.ProductReview{
		{ID: uuid.New(), ProductID: uuid.New(), Rating: 4, Comment: "Чекає на підтвердження", Status: "pending"},
	}
	meta := pagination.Metadata{CurrentPage: 1, PageSize: 10, TotalItems: 1, TotalPages: 1}

	svc.On("GetPendingReviews", mock.Anything, mock.Anything).Return(reviews, meta, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/admin/reviews/pending?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ==========================================
// GetProductFilters
// ==========================================

func TestProductHandler_GetProductFilters_Success(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	fd := &domain.FilterDiscovery{
		MinPrice: 1000,
		MaxPrice: 50000,
		Brands: []domain.BrandFilterOption{
			{ID: uuid.New(), Name: "Brand A", Count: 5},
		},
		Quantities: []domain.QuantityFilterOption{},
		Attributes: []domain.AttributeFilter{},
	}

	svc.On("GetFilters", mock.Anything, mock.Anything, "uk").Return(fd, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products/filters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp FilterDiscoveryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1000, resp.Price.Min)
	assert.Equal(t, 50000, resp.Price.Max)
	assert.Len(t, resp.Brands, 1)
}

func TestProductHandler_GetProductFilters_ServiceError(t *testing.T) {
	svc := &MockProductService{}
	router := newProductRouter(svc)

	svc.On("GetFilters", mock.Anything, mock.Anything, "uk").Return(nil, fmt.Errorf("db error"))

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/products/filters", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ==========================================
// BrandHandler
// ==========================================

func TestBrandHandler_GetBrands_Success(t *testing.T) {
	brandSvc := &MockBrandService{}
	r := gin.New()
	h := NewBrandHandler(brandSvc, &noopLogger{})

	langGroup := r.Group("/api/:lang")
	langGroup.Use(localeMW())
	langGroup.GET("/brands", h.GetBrands)

	brands := []domain.Brand{
		{ID: uuid.New(), Name: "Brand A"},
		{ID: uuid.New(), Name: "Brand B"},
	}
	brandSvc.On("GetAll", mock.Anything).Return(brands, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/brands", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []BrandResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 2)
}

func TestBrandHandler_GetBrands_ServiceError(t *testing.T) {
	brandSvc := &MockBrandService{}
	r := gin.New()
	h := NewBrandHandler(brandSvc, &noopLogger{})

	langGroup := r.Group("/api/:lang")
	langGroup.Use(localeMW())
	langGroup.GET("/brands", h.GetBrands)

	brandSvc.On("GetAll", mock.Anything).Return(nil, fmt.Errorf("db error"))

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/brands", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// RejectReview

func TestProductHandler_RejectReview_Success_EmptyBody(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	reviewID := uuid.New()

	svc.On("RejectReview", mock.Anything, reviewID, (*string)(nil)).Return(nil)

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/reviews/"+reviewID.String()+"/reject", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
}

func TestProductHandler_RejectReview_Success_WithReason(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	reviewID := uuid.New()
	reason := "Spam"

	svc.On("RejectReview", mock.Anything, reviewID, mock.MatchedBy(func(r *string) bool {
		return r != nil && *r == reason
	})).Return(nil)

	body := `{"reason":"Spam"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/reviews/"+reviewID.String()+"/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
}

func TestProductHandler_RejectReview_BadJSON(t *testing.T) {
	svc := &MockProductService{}
	router := newProductAdminRouter(svc)

	reviewID := uuid.New()

	body := `{bad json}`
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/reviews/"+reviewID.String()+"/reject", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

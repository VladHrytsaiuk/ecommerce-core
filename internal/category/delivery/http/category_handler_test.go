package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/category/domain"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newCategoryRouter створює тестовий роутер з middleware що імітує LocaleMiddleware
func newCategoryRouter(svc *MockCategoryService, lang string) *gin.Engine {
	r := gin.New()
	h := NewCategoryHandler(svc, nil, &noopLogger{})

	langGroup := r.Group("/api/:lang")
	langGroup.Use(func(c *gin.Context) {
		l := c.Param("lang")
		if l == "ua" {
			l = "uk"
		}
		if l != "uk" && l != "en" {
			l = "uk"
		}
		c.Set("language", l)
		c.Next()
	})
	langGroup.GET("/categories", h.GetCategories)
	return r
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
// GetCategories
// ==========================================

func TestCategoryHandler_GetCategories_Success(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "uk")

	categories := []domain.Category{
		makeCategoryWithTranslation("uk", "Шампуні"),
		makeCategoryWithTranslation("uk", "Кондиціонери"),
	}
	svc.On("GetList", mock.Anything, "uk", false).Return(categories, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CategoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp, 2)
	assert.Equal(t, "Шампуні", resp[0].Name)
	assert.Equal(t, "Кондиціонери", resp[1].Name)
}

func TestCategoryHandler_GetCategories_EmptyList(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "uk")

	svc.On("GetList", mock.Anything, "uk", false).Return([]domain.Category{}, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CategoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp)
}

func TestCategoryHandler_GetCategories_ServiceError(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "uk")

	svc.On("GetList", mock.Anything, "uk", false).Return(nil, fmt.Errorf("service error"))

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Internal Server Error", resp.Error)
}

func TestCategoryHandler_GetCategories_ResponseContainsParentID(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "uk")

	parentID := uuid.New()
	child := domain.Category{
		ID:       uuid.New(),
		ParentID: &parentID,
		Translations: []domain.CategoryTranslation{
			{LanguageCode: "uk", Name: "Підкатегорія"},
		},
	}
	svc.On("GetList", mock.Anything, "uk", false).Return([]domain.Category{child}, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CategoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp, 1)
	require.NotNil(t, resp[0].ParentID)
	assert.Equal(t, parentID, *resp[0].ParentID)
}

func TestCategoryHandler_GetCategories_EnglishLanguage(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "en")

	categories := []domain.Category{
		makeCategoryWithTranslation("en", "Shampoos"),
	}
	svc.On("GetList", mock.Anything, "en", false).Return(categories, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/en/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CategoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Shampoos", resp[0].Name)
}

func TestCategoryHandler_GetCategories_UaFallsBackToUk(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "ua")

	svc.On("GetList", mock.Anything, "uk", false).Return([]domain.Category{}, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/ua/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertCalled(t, "GetList", mock.Anything, "uk", false)
}

func TestCategoryHandler_GetCategories_NoTranslation_EmptyName(t *testing.T) {
	svc := &MockCategoryService{}
	router := newCategoryRouter(svc, "uk")

	// Категорія без перекладів
	cat := domain.Category{
		ID:           uuid.New(),
		Translations: []domain.CategoryTranslation{},
	}
	svc.On("GetList", mock.Anything, "uk", false).Return([]domain.Category{cat}, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []CategoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "", resp[0].Name)
}

func TestCategoryHandler_ReorderCategories(t *testing.T) {
	svc := &MockCategoryService{}
	r := gin.New()
	h := NewCategoryHandler(svc, nil, &noopLogger{})
	r.PATCH("/api/admin/categories/reorder", h.ReorderCategories)

	id1 := uuid.New()
	id2 := uuid.New()
	svc.On("UpdateOrder", mock.Anything, []uuid.UUID{id1, id2}).Return(nil)

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"ids": []string{id1.String(), id2.String()},
	})
	req, _ := http.NewRequest(http.MethodPatch, "/api/admin/categories/reorder", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Categories reordered successfully", resp["message"])
	svc.AssertExpectations(t)
}

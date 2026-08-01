package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
)

func setupBadgeHandler(svc *MockBadgeService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := NewBadgeHandler(svc, &noopLogger{})

	r.GET("/badges", handler.GetBadges)
	r.POST("/badges", handler.CreateBadge)
	r.PATCH("/badges/:id", handler.UpdateBadge)
	r.DELETE("/badges/:id", handler.DeleteBadge)
	return r
}

func TestBadgeHandler_GetBadges(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	badges := []domain.Badge{
		{ID: 1, Name: domain.LocalizedMap{"uk": "Новинка"}, ColorHex: "#000000", SortOrder: 1, CreatedAt: time.Now()},
	}
	svc.On("GetAll", mock.Anything).Return(badges, nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/badges", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res []AdminBadgeResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, 1, res[0].ID)
}

func TestBadgeHandler_CreateBadge_Success(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	reqBody := CreateBadgeRequest{
		NameUk:    "Знижка",
		NameEn:    "Discount",
		ColorHex:  "#FF0000",
		SortOrder: 2,
	}

	svc.On("Create", mock.Anything, mock.MatchedBy(func(b *domain.Badge) bool {
		return b.Name["uk"] == reqBody.NameUk && b.ColorHex == reqBody.ColorHex
	})).Return(nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/badges", bytes.NewBuffer(toJSON(t, reqBody).Bytes()))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestBadgeHandler_CreateBadge_ValidationFailed(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	reqBody := CreateBadgeRequest{
		NameUk: "", // Required field missing
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/badges", bytes.NewBuffer(toJSON(t, reqBody).Bytes()))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBadgeHandler_UpdateBadge_Success(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	existingBadge := &domain.Badge{
		ID:        1,
		Name:      domain.LocalizedMap{"uk": "Стара назва"},
		ColorHex:  "#000000",
		SortOrder: 1,
	}

	svc.On("GetByID", mock.Anything, 1).Return(existingBadge, nil)

	newName := "Нова назва"
	reqBody := UpdateBadgeRequest{
		NameUk: &newName,
	}

	svc.On("Update", mock.Anything, mock.MatchedBy(func(b *domain.Badge) bool {
		return b.Name["uk"] == newName && b.ID == 1
	})).Return(nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/badges/1", bytes.NewBuffer(toJSON(t, reqBody).Bytes()))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res AdminBadgeResponse
	err := json.Unmarshal(w.Body.Bytes(), &res)
	assert.NoError(t, err)
	assert.Equal(t, newName, res.Name["uk"])
}

func TestBadgeHandler_UpdateBadge_NotFound(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	svc.On("GetByID", mock.Anything, 99).Return(nil, domain.ErrBadgeNotFound)

	reqBody := UpdateBadgeRequest{}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/badges/99", bytes.NewBuffer(toJSON(t, reqBody).Bytes()))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBadgeHandler_DeleteBadge_Success(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	svc.On("Delete", mock.Anything, 1).Return(nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/badges/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBadgeHandler_DeleteBadge_InUse(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	svc.On("Delete", mock.Anything, 1).Return(domain.ErrBadgeInUse)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/badges/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBadgeHandler_DeleteBadge_InvalidID(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/badges/invalid", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBadgeHandler_GetBadges_Error(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	svc.On("GetAll", mock.Anything).Return(nil, errors.New("db error"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/badges", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBadgeHandler_CreateBadge_Error(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	reqBody := CreateBadgeRequest{NameUk: "Тест", ColorHex: "#000"}
	svc.On("Create", mock.Anything, mock.Anything).Return(errors.New("db error"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/badges", bytes.NewBuffer(toJSON(t, reqBody).Bytes()))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBadgeHandler_UpdateBadge_ServiceError(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	existingBadge := &domain.Badge{ID: 1, Name: domain.LocalizedMap{"uk": "Тест"}}
	svc.On("GetByID", mock.Anything, 1).Return(existingBadge, nil)
	svc.On("Update", mock.Anything, mock.Anything).Return(errors.New("db error"))

	reqBody := UpdateBadgeRequest{}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/badges/1", bytes.NewBuffer(toJSON(t, reqBody).Bytes()))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBadgeHandler_DeleteBadge_Error(t *testing.T) {
	svc := new(MockBadgeService)
	r := setupBadgeHandler(svc)

	svc.On("Delete", mock.Anything, 1).Return(errors.New("db error"))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/badges/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

//go:build legacy && ignore
// +build legacy,ignore

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	wishlistHttp "github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/delivery/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/wishlist/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockWishlistService struct {
	mock.Mock
}

func (m *mockWishlistService) GetItems(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string) ([]productDomain.ProductVariation, error) {
	args := m.Called(ctx, userID, sessionID, lang)
	if args.Get(0) != nil {
		return args.Get(0).([]productDomain.ProductVariation), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockWishlistService) AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	args := m.Called(ctx, userID, sessionID, variationID)
	return args.Error(0)
}

func (m *mockWishlistService) RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	args := m.Called(ctx, userID, sessionID, variationID)
	return args.Error(0)
}

func (m *mockWishlistService) SyncSession(ctx context.Context, sessionID string, userID uuid.UUID) error {
	args := m.Called(ctx, sessionID, userID)
	return args.Error(0)
}

func setupTestRouter(svc *mockWishlistService) *gin.Engine {
	logger.Init()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Fake middleware to inject language
	r.Use(func(c *gin.Context) {
		lang := c.Param("lang")
		if lang == "" {
			lang = "uk"
		}
		c.Set("language", lang)

		if uid := c.GetHeader("X-Test-User-ID"); uid != "" {
			c.Set("user_id", uid)
		}
		c.Next()
	})

	handler := wishlistHttp.NewWishlistHandler(svc, logger.Log)
	r.GET("/api/:lang/wishlist", handler.GetWishlist)
	r.POST("/api/:lang/wishlist/:variationId", handler.AddToWishlist)
	r.DELETE("/api/:lang/wishlist/:variationId", handler.RemoveFromWishlist)
	r.POST("/api/:lang/wishlist/sync", handler.SyncWishlist)
	return r
}

func TestWishlistHandler_GetWishlist(t *testing.T) {
	svc := new(mockWishlistService)
	r := setupTestRouter(svc)
	userID := uuid.New()

	t.Run("Success_WithUserID", func(t *testing.T) {
		svc.ExpectedCalls = nil
		variations := []productDomain.ProductVariation{
			{ID: uuid.New(), Price: 100},
		}
		svc.On("GetItems", mock.Anything, &userID, (*string)(nil), "uk").Return(variations, nil).Once()

		req, _ := http.NewRequest(http.MethodGet, "/api/uk/wishlist", nil)
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), variations[0].ID.String())
		svc.AssertExpectations(t)
	})

	t.Run("NoIdentifiers", func(t *testing.T) {
		// New clean router without injected user_id
		rClean := setupTestRouter(svc)
		req, _ := http.NewRequest(http.MethodGet, "/api/uk/wishlist", nil)
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"data":[]}`, w.Body.String())
	})

	t.Run("ServiceError", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("GetItems", mock.Anything, mock.Anything, mock.Anything, "uk").Return(nil, assert.AnError).Once()

		rErr := setupTestRouter(svc)
		req, _ := http.NewRequest(http.MethodGet, "/api/uk/wishlist", nil)
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rErr.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestWishlistHandler_AddToWishlist(t *testing.T) {
	svc := new(mockWishlistService)
	r := setupTestRouter(svc)
	userID := uuid.New()
	variationID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("AddItem", mock.Anything, &userID, (*string)(nil), variationID).Return(nil).Once()

		rClean := setupTestRouter(svc)

		req, _ := http.NewRequest(http.MethodPost, "/api/uk/wishlist/"+variationID.String(), nil)
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/uk/wishlist/not-a-uuid", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("NotFound", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("AddItem", mock.Anything, &userID, (*string)(nil), variationID).Return(domain.ErrVariationNotFound).Once()

		rClean := setupTestRouter(svc)

		req, _ := http.NewRequest(http.MethodPost, "/api/uk/wishlist/"+variationID.String(), nil)
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestWishlistHandler_RemoveFromWishlist(t *testing.T) {
	svc := new(mockWishlistService)
	userID := uuid.New()
	variationID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("RemoveItem", mock.Anything, &userID, (*string)(nil), variationID).Return(nil).Once()

		rClean := setupTestRouter(svc)

		req, _ := http.NewRequest(http.MethodDelete, "/api/uk/wishlist/"+variationID.String(), nil)
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("NotFound", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("RemoveItem", mock.Anything, &userID, (*string)(nil), variationID).Return(domain.ErrItemNotInWishlist).Once()

		rClean := setupTestRouter(svc)

		req, _ := http.NewRequest(http.MethodDelete, "/api/uk/wishlist/"+variationID.String(), nil)
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestWishlistHandler_SyncWishlist(t *testing.T) {
	svc := new(mockWishlistService)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		svc.ExpectedCalls = nil
		svc.On("SyncSession", mock.Anything, "session-123", userID).Return(nil).Once()

		rClean := setupTestRouter(svc)
		body := map[string]string{"session_id": "session-123"}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest(http.MethodPost, "/api/uk/wishlist/sync", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		rClean := setupTestRouter(svc)
		req, _ := http.NewRequest(http.MethodPost, "/api/uk/wishlist/sync", nil)
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("MissingSession", func(t *testing.T) {
		rClean := setupTestRouter(svc)
		req, _ := http.NewRequest(http.MethodPost, "/api/uk/wishlist/sync", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User-ID", userID.String())
		w := httptest.NewRecorder()
		rClean.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

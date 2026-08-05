//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func toJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

func TestCartHandler_AddToCart_Errors(t *testing.T) {
	svc := new(mockCartService)
	router := gin.New()
	handler := NewCartHandler(svc, &noopLogger{})
	router.POST("/cart/items", handler.AddToCart)

	t.Run("Variation Not Found", func(t *testing.T) {
		reqBody := AddToCartRequest{VariationID: uuid.New(), Quantity: 1}
		svc.On("AddItem", mock.Anything, mock.Anything, mock.Anything, reqBody.VariationID, 1).
			Return(domain.ErrVariationNotFound).Once()

		req, _ := http.NewRequest(http.MethodPost, "/cart/items", toJSON(t, reqBody))
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Invalid Quantity", func(t *testing.T) {
		reqBody := AddToCartRequest{VariationID: uuid.New(), Quantity: 1}
		svc.On("AddItem", mock.Anything, mock.Anything, mock.Anything, reqBody.VariationID, 1).
			Return(domain.ErrInvalidQuantity).Once()

		req, _ := http.NewRequest(http.MethodPost, "/cart/items", toJSON(t, reqBody))
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Internal Error", func(t *testing.T) {
		reqBody := AddToCartRequest{VariationID: uuid.New(), Quantity: 1}
		svc.On("AddItem", mock.Anything, mock.Anything, mock.Anything, reqBody.VariationID, 1).
			Return(errors.New("db error")).Once()

		req, _ := http.NewRequest(http.MethodPost, "/cart/items", toJSON(t, reqBody))
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestCartHandler_UpdateCartItem_Errors(t *testing.T) {
	svc := new(mockCartService)
	router := gin.New()
	handler := NewCartHandler(svc, &noopLogger{})
	router.PATCH("/cart/items/:variationId", handler.UpdateCartItem)

	t.Run("Invalid UUID", func(t *testing.T) {
		reqBody := UpdateCartItemRequest{Quantity: 5}
		req, _ := http.NewRequest(http.MethodPatch, "/cart/items/invalid-uuid", toJSON(t, reqBody))
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Item Not Found", func(t *testing.T) {
		vid := uuid.New()
		reqBody := UpdateCartItemRequest{Quantity: 5}
		svc.On("UpdateQuantity", mock.Anything, mock.Anything, mock.Anything, vid, 5).
			Return(domain.ErrCartItemNotFound).Once()

		req, _ := http.NewRequest(http.MethodPatch, "/cart/items/"+vid.String(), toJSON(t, reqBody))
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCartHandler_RemoveFromCart_Errors(t *testing.T) {
	svc := new(mockCartService)
	router := gin.New()
	handler := NewCartHandler(svc, &noopLogger{})
	router.DELETE("/cart/items/:variationId", handler.RemoveFromCart)

	t.Run("Invalid UUID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, "/cart/items/invalid-uuid", nil)
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestCartHandler_ApplyPromoCode_Success(t *testing.T) {
	svc := new(mockCartService)
	router := gin.New()
	handler := NewCartHandler(svc, &noopLogger{})
	router.POST("/cart/promo", handler.ApplyPromoCode)

	t.Run("Success", func(t *testing.T) {
		reqBody := ApplyPromoRequest{Code: "SUMMER10"}
		svc.On("ApplyPromoCode", mock.Anything, mock.Anything, mock.Anything, "SUMMER10", mock.Anything).
			Return(nil).Once()
		svc.On("GetFullCart", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.Cart{}, []productDomain.ProductVariation{}, &domain.ShippingSummary{}, nil, nil, nil).Once()

		req, _ := http.NewRequest(http.MethodPost, "/cart/promo", toJSON(t, reqBody))
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestCartHandler_RemovePromoCode_Success(t *testing.T) {
	svc := new(mockCartService)
	router := gin.New()
	handler := NewCartHandler(svc, &noopLogger{})
	router.DELETE("/cart/promo", handler.RemovePromoCode)

	t.Run("Success", func(t *testing.T) {
		svc.On("RemovePromoCode", mock.Anything, mock.Anything, mock.Anything).
			Return(nil).Once()
		svc.On("GetFullCart", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.Cart{}, []productDomain.ProductVariation{}, &domain.ShippingSummary{}, nil, nil, nil).Once()

		req, _ := http.NewRequest(http.MethodDelete, "/cart/promo", nil)
		req.AddCookie(&http.Cookie{Name: "guest_session", Value: "test-session"})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestCartRoutes(t *testing.T) {
	router := gin.New()
	rg := router.Group("/api")
	svc := new(mockCartService)
	RegisterCartRoutes(rg, rg, svc, &noopLogger{})
	// Ensure no panic
}

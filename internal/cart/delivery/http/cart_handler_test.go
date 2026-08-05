//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/cart/domain"
	discountDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/discount/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	productDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/product/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// --- noopLogger ---

type noopLogger struct{}

func (n *noopLogger) Debug(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Info(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Warn(msg string, fields ...zap.Field)        {}
func (n *noopLogger) Error(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Fatal(msg string, fields ...zap.Field)       {}
func (n *noopLogger) Debugf(template string, args ...interface{}) {}
func (n *noopLogger) Infof(template string, args ...interface{})  {}
func (n *noopLogger) Warnf(template string, args ...interface{})  {}
func (n *noopLogger) Errorf(template string, args ...interface{}) {}
func (n *noopLogger) Fatalf(template string, args ...interface{}) {}
func (n *noopLogger) Debugw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Infow(msg string, kvs ...interface{})        {}
func (n *noopLogger) Warnw(msg string, kvs ...interface{})        {}
func (n *noopLogger) Errorw(msg string, kvs ...interface{})       {}
func (n *noopLogger) Fatalw(msg string, kvs ...interface{})       {}
func (n *noopLogger) With(fields ...zap.Field) logger.Logger      { return n }
func (n *noopLogger) Sync() error                                 { return nil }

// --- Mock CartService ---

type mockCartService struct {
	mock.Mock
}

func (m *mockCartService) GetFullCart(ctx context.Context, userID *uuid.UUID, sessionID *string, lang string) (*domain.Cart, []productDomain.ProductVariation, *domain.ShippingSummary, *discountDomain.PromoCalculationResult, *string, error) {
	args := m.Called(ctx, userID, sessionID, lang)
	var cart *domain.Cart
	if args.Get(0) != nil {
		cart = args.Get(0).(*domain.Cart)
	}
	var variations []productDomain.ProductVariation
	if args.Get(1) != nil {
		variations = args.Get(1).([]productDomain.ProductVariation)
	}
	var shipping *domain.ShippingSummary
	if args.Get(2) != nil {
		shipping = args.Get(2).(*domain.ShippingSummary)
	}
	var promoResult *discountDomain.PromoCalculationResult
	if args.Get(3) != nil {
		promoResult = args.Get(3).(*discountDomain.PromoCalculationResult)
	}
	var promoCodeStr *string
	if args.Get(4) != nil {
		promoCodeStr = args.Get(4).(*string)
	}
	return cart, variations, shipping, promoResult, promoCodeStr, args.Error(5)
}

func (m *mockCartService) AddItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	args := m.Called(ctx, userID, sessionID, variationID, quantity)
	return args.Error(0)
}

func (m *mockCartService) UpdateQuantity(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID, quantity int) error {
	args := m.Called(ctx, userID, sessionID, variationID, quantity)
	return args.Error(0)
}

func (m *mockCartService) RemoveItem(ctx context.Context, userID *uuid.UUID, sessionID *string, variationID uuid.UUID) error {
	args := m.Called(ctx, userID, sessionID, variationID)
	return args.Error(0)
}

func (m *mockCartService) SyncSession(ctx context.Context, sessionID string, userID uuid.UUID) error {
	args := m.Called(ctx, sessionID, userID)
	return args.Error(0)
}

func (m *mockCartService) ApplyPromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string, code string, lang string) error {
	args := m.Called(ctx, userID, sessionID, code, lang)
	return args.Error(0)
}

func (m *mockCartService) RemovePromoCode(ctx context.Context, userID *uuid.UUID, sessionID *string) error {
	args := m.Called(ctx, userID, sessionID)
	return args.Error(0)
}

// --- Helpers ---

func setupCartRouter(svc *mockCartService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	h := NewCartHandler(svc, &noopLogger{})

	lang := r.Group("/api/:lang")
	{
		lang.GET("/cart", h.GetCart)
		lang.POST("/cart/items", h.AddToCart)
		lang.PATCH("/cart/items/:variationId", h.UpdateCartItem)
		lang.DELETE("/cart/items/:variationId", h.RemoveFromCart)
		lang.POST("/cart/sync", h.SyncCart)
	}

	return r
}

// setUserID створює middleware що встановлює user_id в контекст (емуляція AuthMiddleware)
func setUserID(userID uuid.UUID) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Next()
	}
}

// setSessionID створює middleware що встановлює guest_session_id в контекст (емуляція SessionMiddleware)
func setSessionID(sessionID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("guest_session_id", sessionID)
		c.Next()
	}
}

func setupAuthCartRouter(svc *mockCartService, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})

	lang := r.Group("/api/:lang")
	{
		lang.GET("/cart", h.GetCart)
		lang.POST("/cart/items", h.AddToCart)
		lang.PATCH("/cart/items/:variationId", h.UpdateCartItem)
		lang.DELETE("/cart/items/:variationId", h.RemoveFromCart)
		lang.POST("/cart/sync", h.SyncCart)
	}

	return r
}

func setupSessionCartRouter(svc *mockCartService, sessionID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setSessionID(sessionID))

	h := NewCartHandler(svc, &noopLogger{})

	lang := r.Group("/api/:lang")
	{
		lang.GET("/cart", h.GetCart)
		lang.POST("/cart/items", h.AddToCart)
		lang.PATCH("/cart/items/:variationId", h.UpdateCartItem)
		lang.DELETE("/cart/items/:variationId", h.RemoveFromCart)
	}

	return r
}

// --- Tests ---

func TestGetCart_EmptyWithoutIdentifiers(t *testing.T) {
	svc := new(mockCartService)

	emptyCart := &domain.Cart{Items: []domain.CartItem{}}
	shipping := &domain.ShippingSummary{IsFreeShipping: false, Threshold: 60000, RemainingAmount: 60000}
	svc.On("GetFullCart", mock.Anything, (*uuid.UUID)(nil), (*string)(nil), "uk").
		Return(emptyCart, []productDomain.ProductVariation{}, shipping, (*discountDomain.PromoCalculationResult)(nil), (*string)(nil), nil)

	r := setupCartRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/cart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp CartResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Empty(t, resp.Items)
	assert.Equal(t, 0, resp.TotalCount)
	assert.Equal(t, 0, resp.TotalPrice)
}

func TestGetCart_WithUser(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()
	cartID := uuid.New()

	cart := &domain.Cart{
		ID:     cartID,
		UserID: &userID,
		Items: []domain.CartItem{
			{CartID: cartID, VariationID: variationID, Quantity: 2},
		},
	}
	variations := []productDomain.ProductVariation{
		{ID: variationID, Price: 10000, SKU: "TEST-001"},
	}
	shipping := &domain.ShippingSummary{IsFreeShipping: false, Threshold: 60000, RemainingAmount: 40000}

	svc.On("GetFullCart", mock.Anything, &userID, (*string)(nil), "uk").
		Return(cart, variations, shipping, (*discountDomain.PromoCalculationResult)(nil), (*string)(nil), nil)

	r := setupAuthCartRouter(svc, userID)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/cart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp CartResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, 20000, resp.TotalPrice)
	assert.False(t, resp.Shipping.IsFreeShipping)
}

func TestGetCart_ServiceError(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()

	svc.On("GetFullCart", mock.Anything, &userID, (*string)(nil), "uk").
		Return((*domain.Cart)(nil), ([]productDomain.ProductVariation)(nil), (*domain.ShippingSummary)(nil), (*discountDomain.PromoCalculationResult)(nil), (*string)(nil), errors.New("db error"))

	r := setupAuthCartRouter(svc, userID)

	req, _ := http.NewRequest(http.MethodGet, "/api/uk/cart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAddToCart_Success(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()

	svc.On("AddItem", mock.Anything, &userID, (*string)(nil), variationID, 2).Return(nil)

	r := setupAuthCartRouter(svc, userID)

	body, _ := json.Marshal(AddToCartRequest{VariationID: variationID, Quantity: 2})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestAddToCart_InvalidBody(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()

	r := setupAuthCartRouter(svc, userID)

	// Порожній body
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/items", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddToCart_NoIdentifiers(t *testing.T) {
	svc := new(mockCartService)
	r := setupCartRouter(svc)

	variationID := uuid.New()
	body, _ := json.Marshal(AddToCartRequest{VariationID: variationID, Quantity: 1})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddToCart_VariationNotFound(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()

	svc.On("AddItem", mock.Anything, &userID, (*string)(nil), variationID, 1).
		Return(domain.ErrVariationNotFound)

	r := setupAuthCartRouter(svc, userID)

	body, _ := json.Marshal(AddToCartRequest{VariationID: variationID, Quantity: 1})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAddToCart_WithSession(t *testing.T) {
	svc := new(mockCartService)
	sessionID := "test-session-123"

	variationID := uuid.New()

	svc.On("AddItem", mock.Anything, (*uuid.UUID)(nil), &sessionID, variationID, 1).Return(nil)

	r := setupSessionCartRouter(svc, sessionID)

	body, _ := json.Marshal(AddToCartRequest{VariationID: variationID, Quantity: 1})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/items", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestUpdateCartItem_Success(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()

	svc.On("UpdateQuantity", mock.Anything, &userID, (*string)(nil), variationID, 5).Return(nil)

	r := setupAuthCartRouter(svc, userID)

	body, _ := json.Marshal(UpdateCartItemRequest{Quantity: 5})
	req, _ := http.NewRequest(http.MethodPatch, "/api/uk/cart/items/"+variationID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateCartItem_InvalidUUID(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()

	r := setupAuthCartRouter(svc, userID)

	body, _ := json.Marshal(UpdateCartItemRequest{Quantity: 5})
	req, _ := http.NewRequest(http.MethodPatch, "/api/uk/cart/items/not-a-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateCartItem_NotFound(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()

	svc.On("UpdateQuantity", mock.Anything, &userID, (*string)(nil), variationID, 3).
		Return(domain.ErrCartItemNotFound)

	r := setupAuthCartRouter(svc, userID)

	body, _ := json.Marshal(UpdateCartItemRequest{Quantity: 3})
	req, _ := http.NewRequest(http.MethodPatch, "/api/uk/cart/items/"+variationID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRemoveFromCart_Success(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()

	svc.On("RemoveItem", mock.Anything, &userID, (*string)(nil), variationID).Return(nil)

	r := setupAuthCartRouter(svc, userID)

	req, _ := http.NewRequest(http.MethodDelete, "/api/uk/cart/items/"+variationID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRemoveFromCart_NotFound(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	variationID := uuid.New()

	svc.On("RemoveItem", mock.Anything, &userID, (*string)(nil), variationID).
		Return(domain.ErrCartItemNotFound)

	r := setupAuthCartRouter(svc, userID)

	req, _ := http.NewRequest(http.MethodDelete, "/api/uk/cart/items/"+variationID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRemoveFromCart_NoIdentifiers(t *testing.T) {
	svc := new(mockCartService)
	r := setupCartRouter(svc)

	variationID := uuid.New()
	req, _ := http.NewRequest(http.MethodDelete, "/api/uk/cart/items/"+variationID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSyncCart_Success(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	sessionID := "anon-session-456"

	svc.On("SyncSession", mock.Anything, sessionID, userID).Return(nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})
	r.POST("/api/:lang/cart/sync", h.SyncCart)

	body, _ := json.Marshal(SyncCartRequest{SessionID: sessionID})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSyncCart_NoAuth(t *testing.T) {
	svc := new(mockCartService)

	r := setupCartRouter(svc) // без user_id в контексті

	body, _ := json.Marshal(SyncCartRequest{SessionID: "session-123"})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSyncCart_NoSessionID(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})
	r.POST("/api/:lang/cart/sync", h.SyncCart)

	// Порожнє тіло, без куки
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/sync", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSyncCart_SuccessWithCookie(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	sessionID := "anon-session-cookie"

	svc.On("SyncSession", mock.Anything, sessionID, userID).Return(nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})
	r.POST("/api/:lang/cart/sync", h.SyncCart)

	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/sync", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "guest_session", Value: sessionID})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSyncCart_SuccessWithContext(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	sessionID := "anon-session-context"

	svc.On("SyncSession", mock.Anything, sessionID, userID).Return(nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))
	r.Use(setSessionID(sessionID))

	h := NewCartHandler(svc, &noopLogger{})
	r.POST("/api/:lang/cart/sync", h.SyncCart)

	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/sync", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSyncCart_ServiceError(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	sessionID := "session-err"

	svc.On("SyncSession", mock.Anything, sessionID, userID).Return(errors.New("db error"))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})
	r.POST("/api/:lang/cart/sync", h.SyncCart)

	body, _ := json.Marshal(SyncCartRequest{SessionID: sessionID})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApplyPromoCode_Success(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()
	code := "PROMO20"

	svc.On("ApplyPromoCode", mock.Anything, &userID, (*string)(nil), code, "uk").Return(nil)

	cart := &domain.Cart{UserID: &userID}
	variations := []productDomain.ProductVariation{}
	shipping := &domain.ShippingSummary{IsFreeShipping: false}

	svc.On("GetFullCart", mock.Anything, &userID, (*string)(nil), "uk").
		Return(cart, variations, shipping, (*discountDomain.PromoCalculationResult)(nil), &code, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})
	r.POST("/api/:lang/cart/promo", h.ApplyPromoCode)

	body, _ := json.Marshal(ApplyPromoRequest{Code: code})
	req, _ := http.NewRequest(http.MethodPost, "/api/uk/cart/promo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRemovePromoCode_Success(t *testing.T) {
	svc := new(mockCartService)
	userID := uuid.New()

	svc.On("RemovePromoCode", mock.Anything, &userID, (*string)(nil)).Return(nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUserID(userID))

	h := NewCartHandler(svc, &noopLogger{})
	r.DELETE("/api/:lang/cart/promo", h.RemovePromoCode)

	req, _ := http.NewRequest(http.MethodDelete, "/api/uk/cart/promo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

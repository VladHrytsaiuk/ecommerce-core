package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestMain ініціалізує глобальний логер, який використовується в AuthMiddleware
func TestMain(m *testing.M) {
	logger.Init()
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// ==========================================
// MockTokenMaker
// ==========================================

type MockTokenMaker struct {
	mock.Mock
}

func (m *MockTokenMaker) CreateTokenForRole(userID uuid.UUID, role string, duration time.Duration) (string, *token.CustomClaims, error) {
	args := m.Called(userID, role, duration)
	if args.Get(1) == nil {
		return args.String(0), nil, args.Error(2)
	}
	return args.String(0), args.Get(1).(*token.CustomClaims), args.Error(2)
}

func (m *MockTokenMaker) CreateToken(userID uuid.UUID, roleID int, duration time.Duration) (string, *token.CustomClaims, error) {
	args := m.Called(userID, roleID, duration)
	if args.Get(1) == nil {
		return args.String(0), nil, args.Error(2)
	}
	return args.String(0), args.Get(1).(*token.CustomClaims), args.Error(2)
}

func (m *MockTokenMaker) VerifyToken(tokenStr string) (*token.CustomClaims, error) {
	args := m.Called(tokenStr)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*token.CustomClaims), args.Error(1)
}

// ==========================================
// Допоміжна функція
// ==========================================

// newMiddlewareRouter будує тестовий роутер з AuthMiddleware та protected-endpoint
func newMiddlewareRouter(tokenMaker token.Maker) *gin.Engine {
	r := gin.New()
	protected := r.Group("/", AuthMiddleware(tokenMaker))
	protected.GET("/protected", func(c *gin.Context) {
		userID, exists := c.Get(authorizationPayloadKey)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user_id not in context"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": userID.(uuid.UUID).String()})
	})
	return r
}

func makeValidClaims(userID uuid.UUID) *token.CustomClaims {
	return &token.CustomClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
}

// ==========================================
// Auth Middleware Tests
// ==========================================

func TestAuthMiddleware_ValidToken_Passes(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	userID := uuid.New()
	claims := makeValidClaims(userID)

	tokenMaker.On("VerifyToken", "valid-jwt-token").Return(claims, nil)

	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-jwt-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), userID.String())
}

func TestAuthMiddleware_SetsUserIDInContext(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	userID := uuid.New()
	claims := makeValidClaims(userID)

	tokenMaker.On("VerifyToken", mock.Anything).Return(claims, nil)

	var capturedUserID uuid.UUID
	r := gin.New()
	r.GET("/check", AuthMiddleware(tokenMaker), func(c *gin.Context) {
		id, exists := c.Get(authorizationPayloadKey)
		require.True(t, exists)
		capturedUserID = id.(uuid.UUID)
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/check", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, userID, capturedUserID)
}

func TestAuthMiddleware_NoAuthHeader_Returns401(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	// Немає Authorization header
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_WrongAuthType_Returns401(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic, не Bearer
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_MalformedHeader_OnlyOneField_Returns401(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer") // немає токена після "Bearer"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_InvalidToken_Returns401(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	tokenMaker.On("VerifyToken", "invalid-token").
		Return(nil, errors.New("invalid token: signature is invalid"))

	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ShortInvalidTokenReturns401WithoutPanic(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	tokenMaker.On("VerifyToken", "a").Return(nil, errors.New("invalid token"))
	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer a")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ExpiredToken_Returns401(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	tokenMaker.On("VerifyToken", "expired-token").
		Return(nil, errors.New("invalid token: token has expired"))

	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_BearerCaseInsensitive(t *testing.T) {
	// "bearer" (нижній регістр) теж має прийматися
	tokenMaker := &MockTokenMaker{}
	userID := uuid.New()
	claims := makeValidClaims(userID)

	tokenMaker.On("VerifyToken", "valid-token").Return(claims, nil)

	router := newMiddlewareRouter(tokenMaker)

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "bearer valid-token") // нижній регістр
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_BlocksRequest_WithoutCallingHandler(t *testing.T) {
	tokenMaker := &MockTokenMaker{}
	handlerCalled := false

	r := gin.New()
	r.GET("/protected", AuthMiddleware(tokenMaker), func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	// Без Authorization header
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, handlerCalled, "handler не повинен бути викликаний без токена")
}

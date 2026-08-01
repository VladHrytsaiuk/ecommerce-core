package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func newAuthRouterExtra(svc *MockAuthService) *gin.Engine {
	r := gin.New()
	h := NewAuthHandler(svc, nil, &config.Config{}, &noopLogger{})
	r.POST("/api/admin/auth/login", h.AdminLogin)
	r.POST("/api/admin/auth/refresh", h.AdminRefresh)
	r.POST("/auth/setup-password", h.SetupPassword)
	r.POST("/auth/phone-verification/request", h.RequestPhoneVerification)
	r.POST("/auth/phone-verification/confirm", h.ConfirmPhoneVerification)
	return r
}

func TestAuthHandler_AdminLogin(t *testing.T) {
	svc := new(MockAuthService)
	router := newAuthRouterExtra(svc)

	userID := uuid.New()
	authData := makeAuthData(userID)

	t.Run("Success", func(t *testing.T) {
		svc.On("Login", mock.Anything, "admin@test.com", "pass", mock.Anything, mock.Anything, domain.RoleAdmin).
			Return(authData, nil).Once()

		reqBody := LoginRequest{Email: "admin@test.com", Password: "pass"}
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/auth/login", toJSON(t, reqBody))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp AuthResponse
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, authData.AccessToken, resp.AccessToken)
		svc.AssertExpectations(t)
	})
}

func TestAuthHandler_AdminRefresh(t *testing.T) {
	svc := new(MockAuthService)
	router := newAuthRouterExtra(svc)

	userID := uuid.New()
	authData := makeAuthData(userID)

	t.Run("Success", func(t *testing.T) {
		svc.On("Refresh", mock.Anything, "old-refresh", mock.Anything, mock.Anything, domain.RoleAdmin).
			Return(authData, nil).Once()

		reqBody := RefreshRequest{RefreshToken: "old-refresh"}
		req, _ := http.NewRequest(http.MethodPost, "/api/admin/auth/refresh", toJSON(t, reqBody))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestAuthHandler_SetupPassword(t *testing.T) {
	svc := new(MockAuthService)
	router := newAuthRouterExtra(svc)

	userID := uuid.New()
	authData := makeAuthData(userID)

	t.Run("Success", func(t *testing.T) {
		svc.On("SetupPassword", mock.Anything, "token-123", "newPass123", mock.Anything, mock.Anything).
			Return(authData, nil).Once()

		reqBody := SetupPasswordRequest{SetupToken: "token-123", Password: "newPass123"}
		req, _ := http.NewRequest(http.MethodPost, "/auth/setup-password", toJSON(t, reqBody))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestAuthHandler_RequestPhoneVerification(t *testing.T) {
	svc := new(MockAuthService)
	router := newAuthRouterExtra(svc)

	t.Run("Success", func(t *testing.T) {
		svc.On("RequestPhoneVerification", mock.Anything, "+123456", mock.Anything).
			Return("", nil).Once()

		reqBody := VerifyPhoneRequest{Phone: "+123456"}
		req, _ := http.NewRequest(http.MethodPost, "/auth/phone-verification/request", toJSON(t, reqBody))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})
}

func TestAuthHandler_ConfirmPhoneVerification(t *testing.T) {
	svc := new(MockAuthService)
	router := newAuthRouterExtra(svc)

	t.Run("Success", func(t *testing.T) {
		svc.On("ConfirmPhoneVerification", mock.Anything, "+123456", "0000").
			Return(nil).Once()

		reqBody := ConfirmPhoneRequest{Phone: "+123456", Code: "0000"}
		req, _ := http.NewRequest(http.MethodPost, "/auth/phone-verification/confirm", toJSON(t, reqBody))
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		svc.AssertExpectations(t)
	})
}

package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newAuthRouter — допоміжна функція для тестів AuthHandler
func newAuthRouter(svc *MockAuthService) *gin.Engine {
	r := gin.New()
	h := NewAuthHandler(svc, nil, &config.Config{}, &noopLogger{})
	r.POST("/auth/register", h.Register)
	r.POST("/auth/login", h.Login)
	r.POST("/auth/google", h.LoginWithGoogle)
	r.POST("/auth/verify-email", h.VerifyEmail)
	r.POST("/auth/resend-verification", h.ResendVerification)
	r.POST("/auth/forgot-password", h.ForgotPassword)
	r.POST("/auth/reset-password", h.ResetPassword)
	r.POST("/auth/refresh", h.Refresh)
	r.POST("/auth/logout", h.Logout)
	return r
}

func toJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

func makeAuthData(userID uuid.UUID) *domain.AuthResponseData {
	return &domain.AuthResponseData{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		User: domain.User{
			ID:           userID,
			Email:        "test@example.com",
			AuthProvider: "local",
			AvatarURL:    "https://example.com/avatar.jpg",
		},
	}
}

// ==========================================
// Register
// ==========================================

func TestAuthHandler_Register_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	userID := uuid.New()
	svc.On("Register", mock.Anything, mock.Anything).
		Return(makeAuthData(userID), nil)

	body := toJSON(t, map[string]string{
		"first_name": "Jane",
		"last_name":  "Doe",
		"email":      "jane@example.com",
		"password":   "Secret123!",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/register", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Registration successful. Please check your email for verification link.", resp["message"])
	// Токени НЕ повинні бути в тілі відповіді при реєстрації
	assert.Nil(t, resp["access_token"])
	assert.Nil(t, resp["refresh_token"])
}

func TestAuthHandler_Register_InvalidJSON(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	req, _ := http.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_Register_MissingRequiredFields(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	// Email та password відсутні
	body := toJSON(t, map[string]string{
		"first_name": "Jane",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/register", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_Register_EmailConflict(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("Register", mock.Anything, mock.Anything).
		Return(nil, domain.ErrEmailAlreadyExists)

	body := toJSON(t, map[string]string{
		"first_name": "Jane",
		"last_name":  "Doe",
		"email":      "jane@example.com",
		"password":   "Secret123!",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/register", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAuthHandler_Register_PasswordNotInResponse(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	userID := uuid.New()
	authData := &domain.AuthResponseData{
		User: domain.User{
			ID:           userID,
			Email:        "jane@example.com",
			PasswordHash: "bcrypt-secret-hash", // не повинно з'явитися у відповіді
		},
	}
	svc.On("Register", mock.Anything, mock.Anything).Return(authData, nil)

	body := toJSON(t, map[string]string{
		"first_name": "Jane",
		"last_name":  "Doe",
		"email":      "jane@example.com",
		"password":   "Secret123!",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/register", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// password_hash не повинен бути у відповіді
	assert.NotContains(t, w.Body.String(), "bcrypt-secret-hash")
	assert.NotContains(t, w.Body.String(), "password_hash")
}

// ==========================================
// Login
// ==========================================

func TestAuthHandler_Login_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	userID := uuid.New()
	svc.On("Login", mock.Anything, "user@example.com", "password123", mock.Anything, mock.Anything, domain.RoleCustomer).
		Return(makeAuthData(userID), nil)

	body := toJSON(t, map[string]string{
		"email":    "user@example.com",
		"password": "password123",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/login", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "access-token", resp.AccessToken)
	assert.Equal(t, "refresh-token", resp.RefreshToken)
}

func TestAuthHandler_Login_InvalidCredentials(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("Login", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, domain.RoleCustomer).
		Return(nil, domain.ErrInvalidCredentials)

	body := toJSON(t, map[string]string{
		"email":    "user@example.com",
		"password": "wrongpassword",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/login", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Login_EmailNotVerified(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("Login", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, domain.RoleCustomer).
		Return(nil, domain.ErrUserNotVerified)

	body := toJSON(t, map[string]string{
		"email":    "user@example.com",
		"password": "password123",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/login", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuthHandler_Login_InvalidJSON(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	req, _ := http.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// LoginWithGoogle
// ==========================================

func TestAuthHandler_LoginWithGoogle_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	userID := uuid.New()
	svc.On("LoginWithGoogle", mock.Anything, "valid-google-token", mock.Anything, mock.Anything).
		Return(makeAuthData(userID), nil)

	body := toJSON(t, map[string]string{
		"id_token": "valid-google-token",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/google", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "access-token", resp.AccessToken)
}

func TestAuthHandler_LoginWithGoogle_InvalidToken(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("LoginWithGoogle", mock.Anything, "invalid-token", mock.Anything, mock.Anything).
		Return(nil, domain.ErrInvalidGoogleToken)

	body := toJSON(t, map[string]string{
		"id_token": "invalid-token",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/google", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// ==========================================
// VerifyEmail
// ==========================================

func TestAuthHandler_VerifyEmail_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	userID := uuid.New()
	svc.On("VerifyEmail", mock.Anything, "user@example.com", "123456", mock.Anything, mock.Anything).
		Return(makeAuthData(userID), nil)

	body := toJSON(t, map[string]string{
		"email": "user@example.com",
		"code":  "123456",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/verify-email", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken)
}

func TestAuthHandler_VerifyEmail_InvalidCode(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("VerifyEmail", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, domain.ErrInvalidCode)

	body := toJSON(t, map[string]string{
		"email": "user@example.com",
		"code":  "000000",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/verify-email", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_VerifyEmail_ExpiredCode(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("VerifyEmail", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, domain.ErrCodeExpired)

	body := toJSON(t, map[string]string{
		"email": "user@example.com",
		"code":  "123456",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/verify-email", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// ForgotPassword
// ==========================================

func TestAuthHandler_ForgotPassword_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("ForgotPassword", mock.Anything, "user@example.com").Return(nil)

	body := toJSON(t, map[string]string{"email": "user@example.com"})
	req, _ := http.NewRequest(http.MethodPost, "/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_ForgotPassword_InvalidEmail(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	body := toJSON(t, map[string]string{"email": "not-an-email"})
	req, _ := http.NewRequest(http.MethodPost, "/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// ResetPassword
// ==========================================

func TestAuthHandler_ResetPassword_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("ResetPassword", mock.Anything, "user@example.com", "654321", "NewSecret123!").Return(nil)

	body := toJSON(t, map[string]string{
		"email":        "user@example.com",
		"code":         "654321",
		"new_password": "NewSecret123!",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_ResetPassword_InvalidCode(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("ResetPassword", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(domain.ErrInvalidCode)

	body := toJSON(t, map[string]string{
		"email":        "user@example.com",
		"code":         "000000",
		"new_password": "NewSecret123!",
	})
	req, _ := http.NewRequest(http.MethodPost, "/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==========================================
// Refresh
// ==========================================

func TestAuthHandler_Refresh_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	userID := uuid.New()
	svc.On("Refresh", mock.Anything, "old-refresh-token", mock.Anything, mock.Anything, domain.RoleCustomer).
		Return(makeAuthData(userID), nil)

	body := toJSON(t, map[string]string{"refresh_token": "old-refresh-token"})
	req, _ := http.NewRequest(http.MethodPost, "/auth/refresh", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEmpty(t, resp.RefreshToken)
}

func TestAuthHandler_Refresh_Unauthorized(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("Refresh", mock.Anything, mock.Anything, mock.Anything, mock.Anything, domain.RoleCustomer).
		Return(nil, domain.ErrUnauthorized)

	body := toJSON(t, map[string]string{"refresh_token": "bad-token"})
	req, _ := http.NewRequest(http.MethodPost, "/auth/refresh", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthHandler_Refresh_BlockedSession(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("Refresh", mock.Anything, mock.Anything, mock.Anything, mock.Anything, domain.RoleCustomer).
		Return(nil, domain.ErrSessionBlocked)

	body := toJSON(t, map[string]string{"refresh_token": "blocked-token"})
	req, _ := http.NewRequest(http.MethodPost, "/auth/refresh", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ==========================================
// Logout
// ==========================================

func TestAuthHandler_Logout_Success(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	svc.On("Logout", mock.Anything, "refresh-token").Return(nil)

	body := toJSON(t, map[string]string{"refresh_token": "refresh-token"})
	req, _ := http.NewRequest(http.MethodPost, "/auth/logout", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp MessageResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Message, "logged out")
}

func TestAuthHandler_Logout_MissingToken(t *testing.T) {
	svc := &MockAuthService{}
	router := newAuthRouter(svc)

	// Порожній JSON — не пройде binding (refresh_token обов'язковий)
	body := toJSON(t, map[string]string{})
	req, _ := http.NewRequest(http.MethodPost, "/auth/logout", body)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

// ==========================================
// Допоміжні функції
// ==========================================

func testConfig() *config.Config {
	return &config.Config{
		AccessTokenDuration:  15 * time.Minute,
		RefreshTokenDuration: 168 * time.Hour,
		FrontendURL:          "http://localhost:3000",
	}
}

func newTestAuthService(
	userRepo domain.UserRepository,
	sessionRepo domain.SessionRepository,
	verifyCodeRepo domain.VerifyCodeRepository,
	emailProvider *MockEmailProvider,
	tokenMaker domain.AuthService,
) domain.AuthService {
	// Цей хелпер не використовується напряму — дивись newAuthSvc нижче
	return nil
}

func newAuthSvc(
	userRepo *MockUserRepository,
	sessionRepo *MockSessionRepository,
	verifyCodeRepo *MockVerifyCodeRepository,
	emailProvider *MockEmailProvider,
	tokenMaker *MockTokenMaker,
) domain.AuthService {
	return NewAuthService(
		userRepo,
		sessionRepo,
		verifyCodeRepo,
		emailProvider,
		&MockSMSSender{},
		tokenMaker,
		&MockWSNotifyService{},
		testConfig(),
		&noopLogger{},
	)
}

// MockWSNotifyService mock
type MockWSNotifyService struct {
	mock.Mock
}

func (m *MockWSNotifyService) NotifyUser(userID uuid.UUID, data interface{}) error {
	args := m.Called(userID, data)
	return args.Error(0)
}

func makeVerifiedUser() *domain.User {
	return &domain.User{
		ID:              uuid.New(),
		Email:           "test@example.com",
		PasswordHash:    mustHash("password123"),
		FirstName:       "John",
		LastName:        "Doe",
		IsEmailVerified: true,
		IsBlocked:       false,
		RoleID:          domain.RoleCustomer,
	}
}

func mustHash(plain string) string {
	h, err := password.HashPassword(plain)
	if err != nil {
		panic(err)
	}
	return h
}

func stubTokenMaker(m *MockTokenMaker) {
	m.On("CreateToken", mock.Anything, mock.Anything, mock.Anything).
		Return("access-token", &token.CustomClaims{
			RegisteredClaims: jwt.RegisteredClaims{ID: uuid.NewString()},
		}, nil)
}

// ==========================================
// Register
// ==========================================

func TestAuthService_Register_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	emailProvider := &MockEmailProvider{}
	tokenMaker := &MockTokenMaker{}

	svc := newAuthSvc(userRepo, sessionRepo, verifyCodeRepo, emailProvider, tokenMaker)

	user := &domain.User{
		Email:        "new@example.com",
		PasswordHash: "password123", // plain text, буде захешовано
		FirstName:    "Jane",
		LastName:     "Smith",
	}

	userRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	verifyCodeRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendVerificationEmail", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	result, err := svc.Register(context.Background(), user)

	require.NoError(t, err)
	assert.NotNil(t, result)
	// Токени не повертаються після реєстрації — лише дані юзера
	assert.Empty(t, result.AccessToken)
	assert.Empty(t, result.RefreshToken)
	assert.Equal(t, "new@example.com", result.User.Email)
	// Пароль має бути захешований
	assert.NotEqual(t, "password123", result.User.PasswordHash)
	// Дефолтні значення безпеки
	assert.Equal(t, 1, result.User.RoleID)
	assert.False(t, result.User.IsEmailVerified)
	assert.False(t, result.User.IsBlocked)
	assert.Equal(t, "local", result.User.AuthProvider)

	userRepo.AssertExpectations(t)
	verifyCodeRepo.AssertExpectations(t)
	emailProvider.AssertExpectations(t)
}

func TestAuthService_Register_ShortPassword(t *testing.T) {
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	user := &domain.User{
		Email:        "test@example.com",
		PasswordHash: "short",
	}

	result, err := svc.Register(context.Background(), user)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "password must be at least 8 characters")
}

func TestAuthService_Register_EmailAlreadyExists(t *testing.T) {
	userRepo := &MockUserRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	emailProvider := &MockEmailProvider{}

	svc := newAuthSvc(userRepo, &MockSessionRepository{}, verifyCodeRepo, emailProvider, &MockTokenMaker{})

	user := &domain.User{
		Email:        "exists@example.com",
		PasswordHash: "password123",
	}

	userRepo.On("Create", mock.Anything, mock.Anything).Return(domain.ErrEmailAlreadyExists)

	result, err := svc.Register(context.Background(), user)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrEmailAlreadyExists)
}

func TestAuthService_Register_EmailSendFailure_StillSucceeds(t *testing.T) {
	// Навіть якщо email не відправився — реєстрація вважається успішною
	userRepo := &MockUserRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	emailProvider := &MockEmailProvider{}

	svc := newAuthSvc(userRepo, &MockSessionRepository{}, verifyCodeRepo, emailProvider, &MockTokenMaker{})

	user := &domain.User{
		Email:        "test@example.com",
		PasswordHash: "password123",
	}

	userRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	verifyCodeRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendVerificationEmail", mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("smtp error"))

	result, err := svc.Register(context.Background(), user)

	require.NoError(t, err)
	assert.NotNil(t, result)
}

// ==========================================
// Login
// ==========================================

func TestAuthService_Login_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	tokenMaker := &MockTokenMaker{}

	svc := newAuthSvc(userRepo, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, tokenMaker)

	user := makeVerifiedUser()
	userRepo.On("FindByEmail", mock.Anything, user.Email).Return(user, nil)
	stubTokenMaker(tokenMaker)
	sessionRepo.On("DeleteOldest", mock.Anything, user.ID, mock.Anything).Return(nil)
	sessionRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	result, err := svc.Login(context.Background(), user.Email, "password123", "Mozilla", "127.0.0.1", domain.RoleCustomer)

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
	assert.Equal(t, user.Email, result.User.Email)
}

func TestAuthService_Login_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	userRepo.On("FindByEmail", mock.Anything, "ghost@example.com").Return(nil, domain.ErrUserNotFound)

	result, err := svc.Login(context.Background(), "ghost@example.com", "password123", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthService_Login_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	user := makeVerifiedUser()
	userRepo.On("FindByEmail", mock.Anything, user.Email).Return(user, nil)

	result, err := svc.Login(context.Background(), user.Email, "wrongpassword", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthService_Login_EmailNotVerified(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	user := &domain.User{
		ID:              uuid.New(),
		Email:           "unverified@example.com",
		PasswordHash:    mustHash("password123"),
		IsEmailVerified: false,
		RoleID:          domain.RoleCustomer,
	}
	userRepo.On("FindByEmail", mock.Anything, user.Email).Return(user, nil)

	result, err := svc.Login(context.Background(), user.Email, "password123", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrUserNotVerified)
}

func TestAuthService_Login_BlockedUser(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	user := &domain.User{
		ID:              uuid.New(),
		Email:           "blocked@example.com",
		PasswordHash:    mustHash("password123"),
		IsEmailVerified: true,
		IsBlocked:       true,
		RoleID:          domain.RoleCustomer,
	}
	userRepo.On("FindByEmail", mock.Anything, user.Email).Return(user, nil)

	result, err := svc.Login(context.Background(), user.Email, "password123", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestAuthService_Login_DBError_ReturnsInternal(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	userRepo.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, errors.New("connection refused"))

	result, err := svc.Login(context.Background(), "test@example.com", "password123", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInternal)
}

// ==========================================
// VerifyEmail
// ==========================================

func TestAuthService_VerifyEmail_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	tokenMaker := &MockTokenMaker{}

	// Пов'язуємо verifyCodeRepo з userRepo для Atomic
	userRepo.atomicVerifyCodeRepo = verifyCodeRepo

	wsNotifyService := &MockWSNotifyService{}
	svc := NewAuthService(userRepo, sessionRepo, verifyCodeRepo, &MockEmailProvider{}, &MockSMSSender{}, tokenMaker, wsNotifyService, testConfig(), &noopLogger{})

	userID := uuid.New()
	codeID := uuid.New()
	verification := &domain.VerifyCode{
		ID:        codeID,
		UserID:    &userID,
		Code:      "123456",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	user := &domain.User{
		ID:              userID,
		Email:           "test@example.com",
		IsEmailVerified: false,
	}

	verifyCodeRepo.On("FindLastByTarget", mock.Anything, "test@example.com", "email_verification").
		Return(verification, nil)
	userRepo.On("FindByID", mock.Anything, userID).Return(user, nil)
	// Atomic викликається з ctx → fn буде виконано з userRepo та verifyCodeRepo
	userRepo.On("Atomic", mock.Anything).Return(nil)
	userRepo.On("Update", mock.Anything, mock.Anything).Return(nil)
	verifyCodeRepo.On("MarkAsUsed", mock.Anything, codeID).Return(nil)
	stubTokenMaker(tokenMaker)
	sessionRepo.On("DeleteOldest", mock.Anything, userID, mock.Anything).Return(nil)
	sessionRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	// Очікуємо виклик сповіщення
	wsNotifyService.On("NotifyUser", userID, mock.Anything).Return(nil)

	result, err := svc.VerifyEmail(context.Background(), "test@example.com", "123456", "Mozilla", "127.0.0.1")

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
}

func TestAuthService_VerifyEmail_WrongCode(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verification := &domain.VerifyCode{
		Code:      "999999",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)
	verifyCodeRepo.On("UpdateAttempts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	result, err := svc.VerifyEmail(context.Background(), "test@example.com", "123456", "", "")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCode)
}

func TestAuthService_VerifyEmail_AlreadyUsedCode(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verification := &domain.VerifyCode{
		Code:      "123456",
		IsUsed:    true, // вже використаний
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)

	result, err := svc.VerifyEmail(context.Background(), "test@example.com", "123456", "", "")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCode)
}

func TestAuthService_VerifyEmail_ExpiredCode(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verification := &domain.VerifyCode{
		Code:      "123456",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // минув
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)

	result, err := svc.VerifyEmail(context.Background(), "test@example.com", "123456", "", "")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrCodeExpired)
}

func TestAuthService_VerifyEmail_CodeNotFound(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil)

	result, err := svc.VerifyEmail(context.Background(), "test@example.com", "123456", "", "")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrInvalidCode)
}

// ==========================================
// ForgotPassword
// ==========================================

func TestAuthService_ForgotPassword_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	emailProvider := &MockEmailProvider{}

	svc := newAuthSvc(userRepo, &MockSessionRepository{}, verifyCodeRepo, emailProvider, &MockTokenMaker{})

	user := &domain.User{ID: uuid.New(), Email: "user@example.com"}
	userRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(user, nil)
	verifyCodeRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendPasswordResetEmail", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := svc.ForgotPassword(context.Background(), "user@example.com")

	require.NoError(t, err)
	verifyCodeRepo.AssertExpectations(t)
	emailProvider.AssertExpectations(t)
}

func TestAuthService_ForgotPassword_UserNotFound_NoError(t *testing.T) {
	// Безпека: не розкриваємо чи існує email
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	userRepo.On("FindByEmail", mock.Anything, "unknown@example.com").Return(nil, domain.ErrUserNotFound)

	err := svc.ForgotPassword(context.Background(), "unknown@example.com")

	assert.NoError(t, err)
}

// ==========================================
// ResetPassword
// ==========================================

func TestAuthService_ResetPassword_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}

	userRepo.atomicVerifyCodeRepo = verifyCodeRepo

	svc := newAuthSvc(userRepo, sessionRepo, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	userID := uuid.New()
	codeID := uuid.New()
	oldHash := mustHash("oldpassword1")

	verification := &domain.VerifyCode{
		ID:        codeID,
		Code:      "654321",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	user := &domain.User{
		ID:           userID,
		Email:        "user@example.com",
		PasswordHash: oldHash,
	}

	verifyCodeRepo.On("FindLastByTarget", mock.Anything, "user@example.com", "password_reset").
		Return(verification, nil)
	// FindByEmail викликається двічі: спочатку для перевірки "чи не той самий пароль",
	// потім всередині Atomic для оновлення
	userRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(user, nil)
	userRepo.On("Atomic", mock.Anything).Return(nil)
	userRepo.On("Update", mock.Anything, mock.Anything).Return(nil)
	verifyCodeRepo.On("MarkAsUsed", mock.Anything, codeID).Return(nil)
	sessionRepo.On("DeleteAllForUser", mock.Anything, userID).Return(nil)

	err := svc.ResetPassword(context.Background(), "user@example.com", "654321", "newpassword1")

	require.NoError(t, err)
}

func TestAuthService_ResetPassword_WrongCode(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verification := &domain.VerifyCode{
		Code:      "111111",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)
	verifyCodeRepo.On("UpdateAttempts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	err := svc.ResetPassword(context.Background(), "user@example.com", "999999", "newpassword1")

	assert.ErrorIs(t, err, domain.ErrInvalidCode)
}

func TestAuthService_ResetPassword_ExpiredCode(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verification := &domain.VerifyCode{
		Code:      "654321",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(-5 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)

	err := svc.ResetPassword(context.Background(), "user@example.com", "654321", "newpassword1")

	assert.ErrorIs(t, err, domain.ErrCodeExpired)
}

func TestAuthService_ResetPassword_ShortNewPassword(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	verification := &domain.VerifyCode{
		Code:      "654321",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)

	err := svc.ResetPassword(context.Background(), "user@example.com", "654321", "short")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "password must be at least 8 characters")
}

func TestAuthService_ResetPassword_SameAsOldPassword(t *testing.T) {
	userRepo := &MockUserRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	oldPass := "password123"
	oldHash := mustHash(oldPass)

	verification := &domain.VerifyCode{
		Code:      "654321",
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	user := &domain.User{
		ID:           uuid.New(),
		Email:        "user@example.com",
		PasswordHash: oldHash,
	}

	verifyCodeRepo.On("FindLastByTarget", mock.Anything, mock.Anything, mock.Anything).
		Return(verification, nil)
	userRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(user, nil)

	err := svc.ResetPassword(context.Background(), "user@example.com", "654321", oldPass)

	assert.ErrorIs(t, err, domain.ErrNewPasswordSameAsOld)
}

// ==========================================
// ResendVerificationCode
// ==========================================

func TestAuthService_ResendVerificationCode_HappyPath(t *testing.T) {
	userRepo := &MockUserRepository{}
	verifyCodeRepo := &MockVerifyCodeRepository{}
	emailProvider := &MockEmailProvider{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, verifyCodeRepo, emailProvider, &MockTokenMaker{})

	user := &domain.User{
		ID:              uuid.New(),
		Email:           "user@example.com",
		IsEmailVerified: false,
	}
	userRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(user, nil)
	verifyCodeRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	emailProvider.On("SendVerificationEmail", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := svc.ResendVerificationCode(context.Background(), "user@example.com")

	require.NoError(t, err)
}

func TestAuthService_ResendVerificationCode_UserNotFound_NoError(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	userRepo.On("FindByEmail", mock.Anything, mock.Anything).Return(nil, domain.ErrUserNotFound)

	err := svc.ResendVerificationCode(context.Background(), "unknown@example.com")

	assert.NoError(t, err)
}

func TestAuthService_ResendVerificationCode_AlreadyVerified(t *testing.T) {
	userRepo := &MockUserRepository{}
	svc := newAuthSvc(userRepo, &MockSessionRepository{}, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	user := &domain.User{
		ID:              uuid.New(),
		Email:           "user@example.com",
		IsEmailVerified: true, // вже підтверджений
	}
	userRepo.On("FindByEmail", mock.Anything, "user@example.com").Return(user, nil)

	err := svc.ResendVerificationCode(context.Background(), "user@example.com")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already verified")
}

// ==========================================
// Refresh
// ==========================================

func TestAuthService_Refresh_HappyPath(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	tokenMaker := &MockTokenMaker{}
	svc := newAuthSvc(&MockUserRepository{}, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, tokenMaker)

	user := makeVerifiedUser()
	session := &domain.Session{
		ID:           uuid.New(),
		UserID:       user.ID,
		RefreshToken: "old-refresh-token",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		User:         *user,
	}

	sessionRepo.On("FindByToken", mock.Anything, "old-refresh-token").Return(session, nil)
	sessionRepo.On("DeleteByToken", mock.Anything, "old-refresh-token").Return(nil)
	stubTokenMaker(tokenMaker)
	sessionRepo.On("DeleteOldest", mock.Anything, user.ID, mock.Anything).Return(nil)
	sessionRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	result, err := svc.Refresh(context.Background(), "old-refresh-token", "Mozilla", "127.0.0.1", domain.RoleCustomer)

	require.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
}

func TestAuthService_Refresh_SessionNotFound(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	svc := newAuthSvc(&MockUserRepository{}, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	sessionRepo.On("FindByToken", mock.Anything, "bad-token").Return(nil, domain.ErrSessionNotFound)

	result, err := svc.Refresh(context.Background(), "bad-token", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAuthService_Refresh_BlockedSession(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	svc := newAuthSvc(&MockUserRepository{}, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	session := &domain.Session{
		RefreshToken: "blocked-token",
		IsBlocked:    true,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		User:         domain.User{RoleID: domain.RoleCustomer},
	}
	sessionRepo.On("FindByToken", mock.Anything, "blocked-token").Return(session, nil)

	result, err := svc.Refresh(context.Background(), "blocked-token", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrSessionBlocked)
}

func TestAuthService_Refresh_ExpiredSession(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	svc := newAuthSvc(&MockUserRepository{}, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	session := &domain.Session{
		RefreshToken: "expired-token",
		IsBlocked:    false,
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // минула
		User:         domain.User{RoleID: domain.RoleCustomer},
	}
	sessionRepo.On("FindByToken", mock.Anything, "expired-token").Return(session, nil)

	result, err := svc.Refresh(context.Background(), "expired-token", "", "", domain.RoleCustomer)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrSessionExpired)
}

// ==========================================
// Logout
// ==========================================

func TestAuthService_Logout_HappyPath(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	svc := newAuthSvc(&MockUserRepository{}, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	sessionRepo.On("DeleteByToken", mock.Anything, "refresh-token").Return(nil)

	err := svc.Logout(context.Background(), "refresh-token")

	require.NoError(t, err)
	sessionRepo.AssertExpectations(t)
}

func TestAuthService_Logout_SessionNotFound_PropagatesError(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	svc := newAuthSvc(&MockUserRepository{}, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockTokenMaker{})

	sessionRepo.On("DeleteByToken", mock.Anything, mock.Anything).Return(domain.ErrSessionNotFound)

	err := svc.Logout(context.Background(), "bad-token")

	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestAuthService_SessionLimit_CallsDeleteOldest(t *testing.T) {
	userRepo := &MockUserRepository{}
	sessionRepo := &MockSessionRepository{}
	tokenMaker := &MockTokenMaker{}

	// Встановлюємо ліміт 5 у конфігу
	cfg := testConfig()
	cfg.MaxSessions = 5
	svc := NewAuthService(userRepo, sessionRepo, &MockVerifyCodeRepository{}, &MockEmailProvider{}, &MockSMSSender{}, tokenMaker, &MockWSNotifyService{}, cfg, &noopLogger{})

	user := makeVerifiedUser()
	userRepo.On("FindByEmail", mock.Anything, user.Email).Return(user, nil)
	stubTokenMaker(tokenMaker)

	// Очікуємо, що буде видалено найстаріші сесії, залишивши 5-1 = 4 штуки
	sessionRepo.On("DeleteOldest", mock.Anything, user.ID, 4).Return(nil)
	sessionRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	_, err := svc.Login(context.Background(), user.Email, "password123", "", "", domain.RoleCustomer)

	require.NoError(t, err)
	sessionRepo.AssertExpectations(t)
}

func TestAuthService_VerifyEmail_TooManyAttempts(t *testing.T) {
	verifyCodeRepo := &MockVerifyCodeRepository{}
	svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

	// 1. Коли спроб вже 5 або більше, відразу повертає помилку
	verification := &domain.VerifyCode{
		Code:      "999999",
		Attempts:  5,
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, "test@example.com", "email_verification").
		Return(verification, nil)

	result, err := svc.VerifyEmail(context.Background(), "test@example.com", "123456", "", "")
	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrTooManyAttempts)

	// 2. Коли спроба є 5-ою помилковою, повертає помилку
	verification2 := &domain.VerifyCode{
		Code:      "999999",
		Attempts:  4,
		IsUsed:    false,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	verifyCodeRepo.On("FindLastByTarget", mock.Anything, "test2@example.com", "email_verification").
		Return(verification2, nil)
	verifyCodeRepo.On("UpdateAttempts", mock.Anything, mock.Anything, 5).
		Return(nil)

	result2, err2 := svc.VerifyEmail(context.Background(), "test2@example.com", "123456", "", "")
	assert.Nil(t, result2)
	assert.ErrorIs(t, err2, domain.ErrTooManyAttempts)
}

func TestAuthService_ConfirmPhoneVerification(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		verifyCodeRepo := &MockVerifyCodeRepository{}
		svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

		verification := &domain.VerifyCode{
			Code:      "123456",
			IsUsed:    false,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		verifyCodeRepo.On("FindLastByTarget", mock.Anything, "+380999999999", "phone_verification").
			Return(verification, nil)
		verifyCodeRepo.On("MarkAsUsed", mock.Anything, mock.Anything).Return(nil)

		err := svc.ConfirmPhoneVerification(context.Background(), "+380999999999", "123456")
		assert.NoError(t, err)
	})

	t.Run("wrong code updates attempts", func(t *testing.T) {
		verifyCodeRepo := &MockVerifyCodeRepository{}
		svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

		verification := &domain.VerifyCode{
			Code:      "999999",
			Attempts:  2,
			IsUsed:    false,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		verifyCodeRepo.On("FindLastByTarget", mock.Anything, "+380999999999", "phone_verification").
			Return(verification, nil)
		verifyCodeRepo.On("UpdateAttempts", mock.Anything, mock.Anything, 3).Return(nil)

		err := svc.ConfirmPhoneVerification(context.Background(), "+380999999999", "123456")
		assert.ErrorIs(t, err, domain.ErrInvalidCode)
	})

	t.Run("too many attempts", func(t *testing.T) {
		verifyCodeRepo := &MockVerifyCodeRepository{}
		svc := newAuthSvc(&MockUserRepository{}, &MockSessionRepository{}, verifyCodeRepo, &MockEmailProvider{}, &MockTokenMaker{})

		verification := &domain.VerifyCode{
			Code:      "999999",
			Attempts:  4,
			IsUsed:    false,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		verifyCodeRepo.On("FindLastByTarget", mock.Anything, "+380999999999", "phone_verification").
			Return(verification, nil)
		verifyCodeRepo.On("UpdateAttempts", mock.Anything, mock.Anything, 5).Return(nil)

		err := svc.ConfirmPhoneVerification(context.Background(), "+380999999999", "123456")
		assert.ErrorIs(t, err, domain.ErrTooManyAttempts)
	})
}


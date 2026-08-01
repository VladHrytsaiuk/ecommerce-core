package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/email"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
)

// ==========================================
// MockUserRepository
// ==========================================

type MockUserRepository struct {
	mock.Mock
	// atomicVerifyCodeRepo використовується всередині Atomic для передачі в fn
	atomicVerifyCodeRepo domain.VerifyCodeRepository
}

func (m *MockUserRepository) Create(ctx context.Context, user *domain.User) error {
	return m.Called(ctx, user).Error(0)
}

func (m *MockUserRepository) FindAllByRole(ctx context.Context, roleID int) ([]domain.User, error) {
	args := m.Called(ctx, roleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.User), args.Error(1)
}

// ==========================================
// MockAuthService
// ==========================================

type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Register(ctx context.Context, user *domain.User) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, user)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) CreateAdmin(ctx context.Context, user *domain.User) (*domain.User, error) {
	args := m.Called(ctx, user)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockAuthService) ListAdmins(ctx context.Context) ([]domain.User, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.User), args.Error(1)
}

func (m *MockAuthService) DeleteAdmin(ctx context.Context, actorID, userID uuid.UUID) error {
	return m.Called(ctx, actorID, userID).Error(0)
}

func (m *MockAuthService) Login(ctx context.Context, email, password, ua, ip string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, email, password, ua, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) LoginWithGoogle(ctx context.Context, idToken, ua, ip string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, idToken, ua, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) Refresh(ctx context.Context, refreshToken, ua, ip string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, refreshToken, ua, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) Logout(ctx context.Context, refreshToken string) error {
	return m.Called(ctx, refreshToken).Error(0)
}

func (m *MockAuthService) VerifyEmail(ctx context.Context, email, code, ua, ip string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, email, code, ua, ip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) ResendVerificationCode(ctx context.Context, email string) error {
	return m.Called(ctx, email).Error(0)
}

func (m *MockAuthService) ForgotPassword(ctx context.Context, email string) error {
	return m.Called(ctx, email).Error(0)
}

func (m *MockAuthService) ResetPassword(ctx context.Context, email, code, newPass string) error {
	return m.Called(ctx, email, code, newPass).Error(0)
}

func (m *MockAuthService) SetupPassword(ctx context.Context, setupToken, newPassword, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, setupToken, newPassword, userAgent, clientIP)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	args := m.Called(ctx, phone)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, user *domain.User) error {
	return m.Called(ctx, user).Error(0)
}

func (m *MockUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

// Atomic викликає fn з самим собою (UserRepository) та atomicVerifyCodeRepo.
// Якщо мок налаштований повернути nil — fn виконується; інакше повертається помилка.
func (m *MockUserRepository) Atomic(ctx context.Context, fn func(domain.UserRepository, domain.VerifyCodeRepository) error) error {
	args := m.Called(ctx)
	if args.Error(0) == nil && fn != nil {
		return fn(m, m.atomicVerifyCodeRepo)
	}
	return args.Error(0)
}

// ==========================================
// MockSessionRepository
// ==========================================

type MockSessionRepository struct {
	mock.Mock
}

func (m *MockSessionRepository) Create(ctx context.Context, session *domain.Session) error {
	return m.Called(ctx, session).Error(0)
}

func (m *MockSessionRepository) FindByToken(ctx context.Context, tok string) (*domain.Session, error) {
	args := m.Called(ctx, tok)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *MockSessionRepository) DeleteByToken(ctx context.Context, tok string) error {
	return m.Called(ctx, tok).Error(0)
}

func (m *MockSessionRepository) DeleteAllForUser(ctx context.Context, userID uuid.UUID) error {
	return m.Called(ctx, userID).Error(0)
}

func (m *MockSessionRepository) DeleteExpired(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockSessionRepository) DeleteOldest(ctx context.Context, userID uuid.UUID, keepCount int) error {
	return m.Called(ctx, userID, keepCount).Error(0)
}

// ==========================================
// MockVerifyCodeRepository
// ==========================================

type MockVerifyCodeRepository struct {
	mock.Mock
}

func (m *MockVerifyCodeRepository) Create(ctx context.Context, code *domain.VerifyCode) error {
	return m.Called(ctx, code).Error(0)
}

func (m *MockVerifyCodeRepository) FindLastByTarget(ctx context.Context, target string, codeType string) (*domain.VerifyCode, error) {
	args := m.Called(ctx, target, codeType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.VerifyCode), args.Error(1)
}

func (m *MockVerifyCodeRepository) FindByCode(ctx context.Context, codeValue string, codeType string) (*domain.VerifyCode, error) {
	args := m.Called(ctx, codeValue, codeType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.VerifyCode), args.Error(1)
}

func (m *MockVerifyCodeRepository) MarkAsUsed(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockVerifyCodeRepository) DeleteExpired(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockVerifyCodeRepository) UpdateAttempts(ctx context.Context, id uuid.UUID, attempts int) error {
	return m.Called(ctx, id, attempts).Error(0)
}

// ==========================================
// MockUserAddressRepository
// ==========================================

type MockUserAddressRepository struct {
	mock.Mock
}

func (m *MockUserAddressRepository) Create(ctx context.Context, address *domain.UserAddress) error {
	return m.Called(ctx, address).Error(0)
}

func (m *MockUserAddressRepository) FindAllByUserID(ctx context.Context, userID uuid.UUID) ([]domain.UserAddress, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.UserAddress), args.Error(1)
}

func (m *MockUserAddressRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.UserAddress, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.UserAddress), args.Error(1)
}

func (m *MockUserAddressRepository) Update(ctx context.Context, address *domain.UserAddress) error {
	return m.Called(ctx, address).Error(0)
}

func (m *MockUserAddressRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockUserAddressRepository) UnsetDefaultAllForUser(ctx context.Context, userID uuid.UUID) error {
	return m.Called(ctx, userID).Error(0)
}

// Atomic викликає fn з самим собою (UserAddressRepository).
func (m *MockUserAddressRepository) Atomic(ctx context.Context, fn func(domain.UserAddressRepository) error) error {
	args := m.Called(ctx)
	if args.Error(0) == nil && fn != nil {
		return fn(m)
	}
	return args.Error(0)
}

// ==========================================
// MockEmailProvider
// ==========================================

type MockEmailProvider struct {
	mock.Mock
}

func (m *MockEmailProvider) SendVerificationEmail(to, code, link string) error {
	return m.Called(to, code, link).Error(0)
}

func (m *MockEmailProvider) SendPasswordResetEmail(to, code, link string) error {
	return m.Called(to, code, link).Error(0)
}

func (m *MockEmailProvider) SendOrderConfirmationEmail(to string, data email.OrderEmailData) error {
	return m.Called(to, data).Error(0)
}

func (m *MockEmailProvider) SendAdminOrderNotification(to string, data email.OrderEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *MockEmailProvider) SendPaymentReminderEmail(to string, data email.OrderEmailData) error {
	args := m.Called(to, data)
	return args.Error(0)
}

func (m *MockEmailProvider) SendSecurityWarningEmail(to string) error {
	return m.Called(to).Error(0)
}

func (m *MockEmailProvider) SendShipmentCreatedEmail(to string, data email.ShipmentEmailData) error {
	return m.Called(to, data).Error(0)
}

func (m *MockEmailProvider) SendFeedbackEmail(to string, feedbackType string, userEmail string, content string, mediaURL *string) error {
	return m.Called(to, feedbackType, userEmail, content, mediaURL).Error(0)
}

// ==========================================
// MockSMSSender — no-op стаб (не панікує без expectations)
// ==========================================

type MockSMSSender struct{}

func (m *MockSMSSender) SendVerificationCode(_ context.Context, _, _ string) error {
	return nil
}

// ==========================================
// MockTokenMaker
// ==========================================

type MockTokenMaker struct {
	mock.Mock
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
// noopLogger — беззвучний логер для тестів
// ==========================================

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

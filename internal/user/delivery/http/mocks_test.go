package http

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
	"go.uber.org/zap"
)

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

func (m *MockAuthService) Login(ctx context.Context, email, password, userAgent, clientIP string, requiredRole int) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, email, password, userAgent, clientIP, requiredRole)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) LoginWithGoogle(ctx context.Context, idToken, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, idToken, userAgent, clientIP)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) Refresh(ctx context.Context, refreshToken, userAgent, clientIP string, requiredRole int) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, refreshToken, userAgent, clientIP, requiredRole)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) Logout(ctx context.Context, refreshToken string) error {
	return m.Called(ctx, refreshToken).Error(0)
}

func (m *MockAuthService) VerifyEmail(ctx context.Context, email, code string, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, email, code, userAgent, clientIP)
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

func (m *MockAuthService) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	return m.Called(ctx, email, code, newPassword).Error(0)
}

func (m *MockAuthService) SetupPassword(ctx context.Context, setupToken, newPassword, userAgent, clientIP string) (*domain.AuthResponseData, error) {
	args := m.Called(ctx, setupToken, newPassword, userAgent, clientIP)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AuthResponseData), args.Error(1)
}

func (m *MockAuthService) RequestPhoneVerification(ctx context.Context, phone string, userID *uuid.UUID) (string, error) {
	args := m.Called(ctx, phone, userID)
	return args.String(0), args.Error(1)
}

func (m *MockAuthService) ConfirmPhoneVerification(ctx context.Context, phone, code string) error {
	return m.Called(ctx, phone, code).Error(0)
}

// ==========================================
// MockUserService
// ==========================================

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetMe(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserService) UpdateMe(ctx context.Context, userID uuid.UUID, input *domain.UpdateMeInput) (*domain.User, error) {
	args := m.Called(ctx, userID, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserService) DeleteMe(ctx context.Context, userID uuid.UUID) error {
	return m.Called(ctx, userID).Error(0)
}

func (m *MockUserService) GetAddresses(ctx context.Context, userID uuid.UUID) ([]domain.UserAddress, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.UserAddress), args.Error(1)
}

func (m *MockUserService) CreateAddress(ctx context.Context, address *domain.UserAddress) (*domain.UserAddress, error) {
	args := m.Called(ctx, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.UserAddress), args.Error(1)
}

func (m *MockUserService) UpdateAddress(ctx context.Context, userID uuid.UUID, addressID uuid.UUID, address *domain.UserAddress) (*domain.UserAddress, error) {
	args := m.Called(ctx, userID, addressID, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.UserAddress), args.Error(1)
}

func (m *MockUserService) DeleteAddress(ctx context.Context, userID uuid.UUID, addressID uuid.UUID) error {
	return m.Called(ctx, userID, addressID).Error(0)
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
// noopLogger
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

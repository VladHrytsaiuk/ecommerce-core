package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/user/domain"
)

func TestAuthService_SetupPassword(t *testing.T) {
	userRepo := new(MockUserRepository)
	sessionRepo := new(MockSessionRepository)
	verifyRepo := new(MockVerifyCodeRepository)
	emailProv := new(MockEmailProvider)
	tokenMaker := new(MockTokenMaker)
	userRepo.atomicVerifyCodeRepo = verifyRepo
	svc := newAuthSvc(userRepo, sessionRepo, verifyRepo, emailProv, tokenMaker)
	ctx := context.Background()

	userID := uuid.New()
	code := "some-token"
	verification := &domain.VerifyCode{
		ID:        uuid.New(),
		Target:    "target",
		Code:      code,
		Type:      "password_setup",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		IsUsed:    false,
		UserID:    &userID,
	}

	user := &domain.User{
		ID:             userID,
		Email:          "test@example.com",
		PasswordHash:   "",
		RoleID:         domain.RoleCustomer,
	}

	t.Run("Success", func(t *testing.T) {
		verifyRepo.On("FindByCode", ctx, code, "password_setup").Return(verification, nil).Once()
		
		userRepo.On("Atomic", ctx).Return(nil).Once()

		userRepo.On("FindByID", ctx, userID).Return(user, nil).Once()
		userRepo.On("Update", ctx, mock.AnythingOfType("*domain.User")).Return(nil).Once()
		verifyRepo.On("MarkAsUsed", ctx, verification.ID).Return(nil).Once()
		
		tokenMaker.On("CreateToken", userID, domain.RoleCustomer, mock.Anything).Return("access_token", nil, nil).Once()
		tokenMaker.On("CreateToken", userID, domain.RoleCustomer, mock.Anything).Return("refresh_token", nil, nil).Once()
		sessionRepo.On("DeleteOldest", ctx, userID, mock.Anything).Return(nil).Maybe()
		sessionRepo.On("Create", ctx, mock.AnythingOfType("*domain.Session")).Return(nil).Once()

		res, err := svc.SetupPassword(ctx, code, "newPassword123", "UserAgent", "127.0.0.1")
		require.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, "access_token", res.AccessToken)
		verifyRepo.AssertExpectations(t)
		userRepo.AssertExpectations(t)
	})

	t.Run("Invalid Code", func(t *testing.T) {
		verifyRepo.On("FindByCode", ctx, "invalid", "password_setup").Return(nil, nil).Once()
		res, err := svc.SetupPassword(ctx, "invalid", "newPassword123", "UserAgent", "127.0.0.1")
		assert.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("Short Password", func(t *testing.T) {
		verifyRepo.On("FindByCode", ctx, code, "password_setup").Return(verification, nil).Once()
		res, err := svc.SetupPassword(ctx, code, "short", "UserAgent", "127.0.0.1")
		assert.Error(t, err)
		assert.Nil(t, res)
	})
}

func TestAuthService_RequestPhoneVerification(t *testing.T) {
	userRepo := new(MockUserRepository)
	sessionRepo := new(MockSessionRepository)
	verifyRepo := new(MockVerifyCodeRepository)
	emailProv := new(MockEmailProvider)
	tokenMaker := new(MockTokenMaker)
	svc := newAuthSvc(userRepo, sessionRepo, verifyRepo, emailProv, tokenMaker)
	ctx := context.Background()

	phone := "+380501234567"
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		verifyRepo.On("Create", ctx, mock.AnythingOfType("*domain.VerifyCode")).Return(nil).Once()

		code, err := svc.RequestPhoneVerification(ctx, phone, &userID)
		require.NoError(t, err)
		// in dev mode, code is returned directly
		assert.NotEmpty(t, code)
		verifyRepo.AssertExpectations(t)
	})
}



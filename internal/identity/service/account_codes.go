package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
)

// RequestEmailVerification sends a code to the signed-in account's address.
// Registration sends the first one; this is for another.
func (s *AuthService) RequestEmailVerification(ctx context.Context, userID uuid.UUID) (domain.CodeRequest, error) {
	_, email, err := s.accountToVerify(ctx, userID)
	if err != nil {
		return domain.CodeRequest{}, err
	}
	return s.codes.issue(ctx, s.now().UTC(), domain.CodeChannelEmail, domain.CodePurposeVerifyEmail, email, false, nil)
}

// ConfirmEmail marks the signed-in account's address verified with the code
// sent to it.
func (s *AuthService) ConfirmEmail(ctx context.Context, command domain.ConfirmEmailCommand) error {
	user, email, err := s.accountToVerify(ctx, command.UserID)
	if err != nil {
		return err
	}
	return s.codes.consume(ctx, s.now().UTC(), domain.CodeChannelEmail, domain.CodePurposeVerifyEmail, email, command.Code, func(txCtx context.Context) error {
		verified, err := s.codes.accounts.VerifyEmail(txCtx, user.ID, email)
		if err != nil {
			return err
		}
		if !verified {
			// The address changed after the code was sent to the old one.
			return domain.ErrInvalidCode
		}
		return nil
	})
}

func (s *AuthService) accountToVerify(ctx context.Context, userID uuid.UUID) (*domain.User, string, error) {
	if !s.codesAvailable(domain.CodeChannelEmail) || s.users == nil {
		return nil, "", domain.ErrCodesUnavailable
	}
	user, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, domain.ErrUserNotFound) || (err == nil && !canSignIn(user)) {
		return nil, "", domain.ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", err
	}
	if user.Email == nil {
		return nil, "", domain.ErrNoEmailToVerify
	}
	if user.EmailVerified {
		return nil, "", domain.ErrEmailAlreadyVerified
	}
	email, ok := normalizeEmail(*user.Email)
	if !ok {
		return nil, "", domain.ErrInvalidCodeDestination
	}
	return user, email, nil
}

// RequestPasswordReset sends a code for setting a new password to the address,
// if it has an account that can sign in. The answer is the same either way.
func (s *AuthService) RequestPasswordReset(ctx context.Context, command domain.RequestPasswordResetCommand) (domain.CodeRequest, error) {
	if err := s.passwordResetAvailable(); err != nil {
		return domain.CodeRequest{}, err
	}
	email, ok := normalizeEmail(command.Email)
	if !ok {
		return domain.CodeRequest{}, domain.ErrInvalidCodeDestination
	}
	return s.codes.issue(ctx, s.now().UTC(), domain.CodeChannelEmail, domain.CodePurposeResetPassword, email, false, func(txCtx context.Context) (bool, error) {
		user, err := s.codes.accounts.FindByEmail(txCtx, email)
		if errors.Is(err, domain.ErrUserNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return canSignIn(user), nil
	})
}

// ResetPassword sets a new password with the code sent to the address, and
// signs in. Every other sign-in of the account ends: a reset is what someone
// does when they think another person has their password. The address is
// marked verified, because the code was read there.
func (s *AuthService) ResetPassword(ctx context.Context, command domain.ResetPasswordCommand) (domain.Session, error) {
	if err := s.passwordResetAvailable(); err != nil {
		return domain.Session{}, err
	}
	email, ok := normalizeEmail(command.Email)
	if !ok {
		return domain.Session{}, domain.ErrInvalidCode
	}
	// Checked before the code, so a rejected password does not use up an
	// attempt.
	if !validPassword(command.Password) {
		return domain.Session{}, domain.ErrInvalidPassword
	}
	hash, err := password.HashPassword(command.Password)
	if err != nil {
		return domain.Session{}, err
	}
	now := s.now().UTC()
	var user *domain.User
	err = s.codes.consume(ctx, now, domain.CodeChannelEmail, domain.CodePurposeResetPassword, email, command.Code, func(txCtx context.Context) error {
		found, err := s.codes.accounts.FindByEmail(txCtx, email)
		if errors.Is(err, domain.ErrUserNotFound) || (err == nil && !canSignIn(found)) {
			return domain.ErrInvalidCode
		}
		if err != nil {
			return err
		}
		if err := s.codes.accounts.SetPassword(txCtx, found.ID, hash); err != nil {
			return err
		}
		if err := s.endEverySignIn(txCtx, found.ID, now); err != nil {
			return err
		}
		found.PasswordHash, found.EmailVerified = hash, true
		user = found
		return nil
	})
	if err != nil {
		return domain.Session{}, err
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

func (s *AuthService) passwordResetAvailable() error {
	if s.passwordSignInDisabled {
		return domain.ErrSignInMethodDisabled
	}
	if !s.codesAvailable(domain.CodeChannelEmail) {
		return domain.ErrCodesUnavailable
	}
	return nil
}

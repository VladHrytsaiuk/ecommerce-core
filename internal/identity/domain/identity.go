package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrInvalidPassword            = errors.New("invalid password")
	ErrUserNotFound               = errors.New("identity user not found")
	ErrEmailAlreadyExists         = errors.New("identity email already exists")
	ErrPhoneAlreadyExists         = errors.New("identity phone already exists")
	ErrOAuthIdentityNotFound      = errors.New("OAuth identity not found")
	ErrOAuthIdentityAlreadyLinked = errors.New("OAuth identity is already linked")
	ErrOAuthProviderUnavailable   = errors.New("OAuth provider is unavailable")
	ErrInvalidOAuthState          = errors.New("invalid or expired OAuth state")
	ErrOAuthAccountLinkRequired   = errors.New("OAuth account linking is required")
	ErrInvalidProfile             = errors.New("invalid profile")
	ErrProfileConflict            = errors.New("profile update conflict")
	ErrProfileNotFound            = errors.New("profile not found")
)

// UserReader is the narrow Core-owned port required by login. It deliberately
// has no dependency on the retired legacy User repository.
type UserReader interface {
	FindByEmail(context.Context, string) (*User, error)
}

type LoginResult struct {
	AccessToken string
	ExpiresAt   time.Time
	Role        string
}

type Service interface {
	Login(context.Context, string, string) (*LoginResult, error)
}

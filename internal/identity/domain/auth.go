package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AuthService defines application use cases. OAuth provider protocol handling
// remains behind OAuthProvider adapters selected by Bootstrap.
type AuthService interface {
	RegisterPassword(context.Context, RegisterPasswordCommand) (Session, error)
	LoginPassword(context.Context, PasswordLoginCommand) (Session, error)
	BeginOAuth(context.Context, BeginOAuthCommand) (OAuthAuthorization, error)
	CompleteOAuth(context.Context, CompleteOAuthCommand) (Session, error)
}

type ProfileService interface {
	GetProfile(context.Context, uuid.UUID) (*Profile, error)
	UpdateProfile(context.Context, uuid.UUID, []byte) (*Profile, error)
}

type RegisterPasswordCommand struct {
	Email    *string
	Phone    *string
	Password string
}

type PasswordLoginCommand struct {
	// Login is a normalized email address or phone number. The application
	// decides which form it is; repositories do not expose provider semantics.
	Login    string
	Password string
}

type Session struct {
	AccessToken string
	ExpiresAt   time.Time
	UserID      uuid.UUID
	Role        Role
}

type BeginOAuthCommand struct {
	Provider     string
	RedirectURI  string
	State        string
	Nonce        string
	CodeVerifier string
}

type CompleteOAuthCommand struct {
	Provider    string
	RedirectURI string
	Code        string
	State       string
}

type OAuthAuthorizationRequest struct {
	RedirectURI  string
	State        string
	Nonce        string
	CodeVerifier string
}

type OAuthAuthorization struct {
	RedirectURL string
}

type OAuthCodeExchange struct {
	RedirectURI  string
	Code         string
	CodeVerifier string
	Nonce        string
}

// VerifiedOAuthIdentity is returned only after the adapter verifies the
// provider response and token/issuer/audience requirements.
type VerifiedOAuthIdentity struct {
	Provider      string
	Subject       string
	Email         *string
	EmailVerified bool
}

type OAuthProvider interface {
	Code() string
	BeginAuthorization(context.Context, OAuthAuthorizationRequest) (OAuthAuthorization, error)
	ExchangeCode(context.Context, OAuthCodeExchange) (VerifiedOAuthIdentity, error)
}

type OAuthProviderRegistry interface {
	Get(code string) (OAuthProvider, bool)
}

// UserRepository is the write-capable persistence port for the strict Core
// user aggregate. Implementations belong in repository adapters.
type UserRepository interface {
	Create(ctx context.Context, user NewUser) (*User, error)
	FindByLogin(ctx context.Context, login string) (*User, error)
	FindByID(ctx context.Context, userID uuid.UUID) (*User, error)
}

type NewUser struct {
	Email        *string
	Phone        *string
	PasswordHash string
	Role         Role
	Status       UserStatus
}

type OAuthIdentityRepository interface {
	FindByProviderSubject(ctx context.Context, provider, subject string) (*OAuthIdentity, error)
	Create(ctx context.Context, identity OAuthIdentity) (*OAuthIdentity, error)
}

// OAuthAttemptStore owns short-lived, one-time authorization state. State is
// accepted as plaintext at this port boundary but persistence stores only a
// digest, so a database read cannot be replayed in a browser callback.
type OAuthAttemptStore interface {
	Create(ctx context.Context, attempt OAuthAttempt) error
	Consume(ctx context.Context, provider, state string, now time.Time) (*OAuthAttempt, error)
}

type OAuthAttempt struct {
	Provider     string
	State        string
	RedirectURI  string
	Nonce        string
	CodeVerifier string
	ExpiresAt    time.Time
}

// AuthTransaction makes the local user + external identity link atomic. OAuth
// HTTP calls always happen before this transaction is entered.
type AuthTransaction interface {
	WithinTransaction(ctx context.Context, fn func(UserRepository, OAuthIdentityRepository) error) error
}

// ProfileRepository is available only when the user_profiles module is
// enabled. Attributes must be validated against ProfilePolicy before Upsert.
type ProfileRepository interface {
	FindByUserID(ctx context.Context, userID uuid.UUID) (*Profile, error)
	Upsert(ctx context.Context, profile Profile) (*Profile, error)
}

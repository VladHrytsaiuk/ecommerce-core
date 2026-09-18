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
	// RefreshSession exchanges a refresh token for a new session and a new
	// refresh token; the presented one stops working.
	RefreshSession(context.Context, string) (Session, error)
	// RevokeSession ends the sign-in a refresh token belongs to.
	RevokeSession(context.Context, string) error
	// RequestSignInCode sends a one-time code to an address.
	RequestSignInCode(context.Context, RequestSignInCodeCommand) (CodeRequest, error)
	// VerifySignInCode exchanges a code for a session, registering the address
	// if it has no account yet.
	VerifySignInCode(context.Context, VerifySignInCodeCommand) (Session, error)
	// RequestEmailVerification sends a code to a signed-in account's address.
	RequestEmailVerification(context.Context, uuid.UUID) (CodeRequest, error)
	// ConfirmEmail marks the account's address verified with that code.
	ConfirmEmail(context.Context, ConfirmEmailCommand) error
	// RequestPasswordReset sends a code for setting a new password.
	RequestPasswordReset(context.Context, RequestPasswordResetCommand) (CodeRequest, error)
	// ResetPassword sets the new password with that code and signs in.
	ResetPassword(context.Context, ResetPasswordCommand) (Session, error)
	// Account is what a signed-in customer's own client needs to know about
	// their account: which contacts it has, and which are verified.
	Account(context.Context, uuid.UUID) (Account, error)
}

// Account is a signed-in account as its own client sees it. It carries no
// password hash and nothing about any other account.
type Account struct {
	UserID        uuid.UUID
	Role          Role
	Email         *string
	Phone         *string
	EmailVerified bool
	PhoneVerified bool
	// HasPassword says whether this account can sign in with a password, which
	// a client needs in order to offer setting one.
	HasPassword bool
}

type ProfileService interface {
	GetProfile(context.Context, uuid.UUID) (*Profile, error)
	UpdateProfile(context.Context, uuid.UUID, []byte) (*Profile, error)
}

type RegisterPasswordCommand struct {
	Email          *string
	Phone          *string
	Password       string
	GuestSessionID *uuid.UUID
}

type PasswordLoginCommand struct {
	// Login is a normalized email address or phone number. The application
	// decides which form it is; repositories do not expose provider semantics.
	Login          string
	Password       string
	GuestSessionID *uuid.UUID
}

type Session struct {
	AccessToken string
	ExpiresAt   time.Time
	UserID      uuid.UUID
	Role        Role
	// RefreshToken is empty where no refresh store is configured. When set it
	// is shown to the client once and stored only as a hash.
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type BeginOAuthCommand struct {
	Provider     string
	RedirectURI  string
	State        string
	Nonce        string
	CodeVerifier string
}

type CompleteOAuthCommand struct {
	Provider       string
	RedirectURI    string
	Code           string
	State          string
	GuestSessionID *uuid.UUID
}

// UserLoginObserver is an optional application event hook. Its implementation
// is selected in Bootstrap, so Identity never imports optional modules.
type UserLoginObserver interface {
	OnUserLogin(context.Context, uuid.UUID, *uuid.UUID) error
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

// VerificationStatusReader deliberately exposes no user contact values,
// password hash, session or OAuth material to consuming modules.
type VerificationStatusReader interface {
	GetVerificationStatus(ctx context.Context, userID uuid.UUID) (UserVerificationStatus, error)
}

type UserVerificationStatus struct {
	EmailVerified bool
	PhoneVerified bool
}

type NewUser struct {
	Email         *string
	Phone         *string
	EmailVerified bool
	PhoneVerified bool
	PasswordHash  string
	Role          Role
	Status        UserStatus
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

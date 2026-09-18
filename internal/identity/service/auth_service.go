// Package service contains identity application workflows. It depends only on
// domain ports and the local token/password primitives, never on OAuth SDKs.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

type AuthService struct {
	users       domain.UserRepository
	identities  domain.OAuthIdentityRepository
	attempts    domain.OAuthAttemptStore
	transaction domain.AuthTransaction
	providers   domain.OAuthProviderRegistry
	tokens      token.Maker
	accessTTL   time.Duration
	attemptTTL  time.Duration
	observer    domain.UserLoginObserver
	now         func() time.Time
	// refreshTokens is nil unless WithRefreshTokens enabled them.
	refreshTokens domain.RefreshTokenStore
	refreshTTL    time.Duration
	// passwordSignInDisabled is false by default, so a service built without
	// WithPasswordSignIn keeps the behaviour it always had.
	passwordSignInDisabled bool
	// codes is nil unless WithCodes made one-time codes available.
	// accounts reads and changes account contacts; WithAccounts attaches it.
	accounts domain.CodeAccounts
	codes    *oneTimeCodes
	// codeSignIn holds the channels sign-in by code is enabled on.
	codeSignIn map[domain.CodeChannel]bool
}

// WithPasswordSignIn enables or disables registration and sign-in with a
// password. Disabling it here, and not only by leaving the routes out, means
// no other caller can reach a method the store turned off.
func (s *AuthService) WithPasswordSignIn(enabled bool) *AuthService {
	s.passwordSignInDisabled = !enabled
	return s
}

// WithUserLoginObserver attaches an optional module subscriber assembled by
// Bootstrap. It preserves the core constructor for deployments without it.
func (s *AuthService) WithUserLoginObserver(observer domain.UserLoginObserver) *AuthService {
	s.observer = observer
	return s
}

func NewAuthService(users domain.UserRepository, identities domain.OAuthIdentityRepository, attempts domain.OAuthAttemptStore, transaction domain.AuthTransaction, providers domain.OAuthProviderRegistry, tokens token.Maker, accessTTL, attemptTTL time.Duration) *AuthService {
	return &AuthService{users: users, identities: identities, attempts: attempts, transaction: transaction, providers: providers, tokens: tokens, accessTTL: accessTTL, attemptTTL: attemptTTL, now: time.Now}
}

func (s *AuthService) RegisterPassword(ctx context.Context, command domain.RegisterPasswordCommand) (domain.Session, error) {
	if s.passwordSignInDisabled {
		return domain.Session{}, domain.ErrSignInMethodDisabled
	}
	if s.users == nil || s.tokens == nil || s.accessTTL <= 0 {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	email, phone := normalizeContact(command.Email), normalizeContact(command.Phone)
	if email == nil && phone == nil {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	if email != nil {
		normalized, ok := normalizeEmail(*email)
		if !ok {
			return domain.Session{}, domain.ErrInvalidEmail
		}
		email = &normalized
	}
	if phone != nil {
		// Stored in the one form sign-in by code looks numbers up in, so the
		// same number is never two accounts.
		normalized, ok := normalizePhone(*phone)
		if !ok {
			return domain.Session{}, domain.ErrInvalidPhone
		}
		phone = &normalized
	}
	if !validPassword(command.Password) {
		return domain.Session{}, domain.ErrInvalidPassword
	}
	hash, err := password.HashPassword(command.Password)
	if err != nil {
		return domain.Session{}, err
	}
	newUser := domain.NewUser{Email: email, Phone: phone, PasswordHash: hash, Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	var user *domain.User
	if email != nil && s.codesAvailable(domain.CodeChannelEmail) {
		// The account and the code that verifies its address are created in one
		// transaction, so there is never an account whose code was lost. The
		// code is unthrottled: registering happens once per address, so it
		// cannot be repeated to flood a mailbox, and a registration must not
		// fail because someone recently asked for codes to the same address.
		_, err = s.codes.issue(ctx, s.now().UTC(), domain.CodeChannelEmail, domain.CodePurposeVerifyEmail, *email, true, func(txCtx context.Context) (bool, error) {
			created, createErr := s.users.Create(txCtx, newUser)
			user = created
			return true, createErr
		})
	} else {
		user, err = s.users.Create(ctx, newUser)
	}
	if err != nil {
		return domain.Session{}, err
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

// passwordCostEqualizer is a bcrypt hash of a random value, computed once on
// first use. Returning early for an unknown or disabled login skipped bcrypt
// entirely and answered orders of magnitude faster than a real password check,
// which is a reliable oracle for whether an address is registered. Comparing
// against this hash instead keeps every attempt's cost the same.
var passwordCostEqualizer = sync.OnceValue(func() string {
	hash, err := password.HashPassword(uuid.NewString())
	if err != nil {
		// GenerateFromPassword only rejects an out-of-range cost or an input
		// over 72 bytes, and a UUID is neither. Reaching this means bcrypt
		// itself is unusable, so no login could succeed in any case.
		panic("identity: bcrypt unavailable for password cost equalizer: " + err.Error())
	}
	return hash
})

func (s *AuthService) LoginPassword(ctx context.Context, command domain.PasswordLoginCommand) (domain.Session, error) {
	if s.passwordSignInDisabled {
		return domain.Session{}, domain.ErrSignInMethodDisabled
	}
	if s.users == nil || s.tokens == nil || s.accessTTL <= 0 {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	user, err := s.users.FindByLogin(ctx, loginKey(command.Login))
	usable := err == nil && user != nil && user.Status == domain.UserStatusActive &&
		validRole(user.Role) && user.PasswordHash != ""

	// Always run the comparison, including when no account can match, so the
	// response time never distinguishes a registered address from an unknown
	// one. Both outcomes still collapse into the same opaque error.
	hash := passwordCostEqualizer()
	if usable {
		hash = user.PasswordHash
	}
	if passwordErr := password.CheckPassword(command.Password, hash); !usable || passwordErr != nil {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

func (s *AuthService) BeginOAuth(ctx context.Context, command domain.BeginOAuthCommand) (domain.OAuthAuthorization, error) {
	provider, err := s.provider(command.Provider)
	if err != nil || s.attempts == nil || s.attemptTTL <= 0 {
		return domain.OAuthAuthorization{}, domain.ErrOAuthProviderUnavailable
	}
	if strings.TrimSpace(command.RedirectURI) == "" {
		return domain.OAuthAuthorization{}, domain.ErrInvalidOAuthState
	}
	state, err := secureRandomValue()
	if err != nil {
		return domain.OAuthAuthorization{}, err
	}
	nonce, err := secureRandomValue()
	if err != nil {
		return domain.OAuthAuthorization{}, err
	}
	codeVerifier, err := secureRandomValue()
	if err != nil {
		return domain.OAuthAuthorization{}, err
	}
	now := s.now().UTC()
	attempt := domain.OAuthAttempt{Provider: provider.Code(), State: state, RedirectURI: command.RedirectURI, Nonce: nonce, CodeVerifier: codeVerifier, ExpiresAt: now.Add(s.attemptTTL)}
	if err := s.attempts.Create(ctx, attempt); err != nil {
		return domain.OAuthAuthorization{}, err
	}
	return provider.BeginAuthorization(ctx, domain.OAuthAuthorizationRequest{RedirectURI: attempt.RedirectURI, State: state, Nonce: nonce, CodeVerifier: codeVerifier})
}

func (s *AuthService) CompleteOAuth(ctx context.Context, command domain.CompleteOAuthCommand) (domain.Session, error) {
	provider, err := s.provider(command.Provider)
	if err != nil || s.attempts == nil || s.identities == nil || s.users == nil || s.transaction == nil || s.tokens == nil {
		return domain.Session{}, domain.ErrOAuthProviderUnavailable
	}
	attempt, err := s.attempts.Consume(ctx, provider.Code(), command.State, s.now().UTC())
	if err != nil {
		return domain.Session{}, err
	}
	if command.RedirectURI != attempt.RedirectURI || strings.TrimSpace(command.Code) == "" {
		return domain.Session{}, domain.ErrInvalidOAuthState
	}
	// Network I/O happens before the local transaction that creates/links data.
	verified, err := provider.ExchangeCode(ctx, domain.OAuthCodeExchange{RedirectURI: attempt.RedirectURI, Code: command.Code, CodeVerifier: attempt.CodeVerifier, Nonce: attempt.Nonce})
	if err != nil || verified.Provider != provider.Code() || strings.TrimSpace(verified.Subject) == "" {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	identity, err := s.identities.FindByProviderSubject(ctx, verified.Provider, verified.Subject)
	if err == nil && identity != nil {
		user, findErr := s.users.FindByID(ctx, identity.UserID)
		if findErr != nil || user.Status != domain.UserStatusActive {
			return domain.Session{}, domain.ErrInvalidCredentials
		}
		return s.finishLogin(ctx, user, command.GuestSessionID)
	}
	if err != nil && !errors.Is(err, domain.ErrOAuthIdentityNotFound) {
		return domain.Session{}, err
	}
	if verified.Email == nil || !verified.EmailVerified {
		return domain.Session{}, domain.ErrOAuthAccountLinkRequired
	}
	if existing, findErr := s.users.FindByLogin(ctx, *verified.Email); findErr == nil && existing != nil {
		session, linked, linkErr := s.linkVerifiedAccount(ctx, existing, verified, command.GuestSessionID)
		if linked {
			return session, linkErr
		}
		if linkErr != nil {
			return domain.Session{}, linkErr
		}
		// The address was detached from an account that never proved it; the
		// account below is created for the person Google says owns it.
	} else if findErr != nil && !errors.Is(findErr, domain.ErrUserNotFound) {
		return domain.Session{}, findErr
	}

	var user *domain.User
	err = s.transaction.WithinTransaction(ctx, func(users domain.UserRepository, identities domain.OAuthIdentityRepository) error {
		created, createErr := users.Create(ctx, domain.NewUser{Email: normalizeContact(verified.Email), EmailVerified: verified.EmailVerified, Role: domain.RoleCustomer, Status: domain.UserStatusActive})
		if createErr != nil {
			if errors.Is(createErr, domain.ErrEmailAlreadyExists) {
				return domain.ErrOAuthAccountLinkRequired
			}
			return createErr
		}
		_, createErr = identities.Create(ctx, domain.OAuthIdentity{ID: uuid.New(), UserID: created.ID, Provider: verified.Provider, Subject: verified.Subject, ProviderEmail: normalizeContact(verified.Email), EmailVerified: verified.EmailVerified})
		if createErr != nil {
			return createErr
		}
		user = created
		return nil
	})
	if err != nil {
		return domain.Session{}, err
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

// linkVerifiedAccount signs in to an existing account with the provider's
// identity, linking the two. linked is false when the caller should carry on
// and create an account instead.
//
// It links only when both sides have verified the address: the provider says
// the person signing in owns it, and the local account proved it with a code or
// an earlier provider sign-in. An account that never proved the address may
// have been registered by someone who does not own it, so:
//
//   - it proved another contact instead: that person owns the account, and the
//     address is detached from it, as it is for a sign-in code;
//   - it proved nothing, or it is not a customer's: linking stays a decision
//     for whoever holds the account, and the callback answers 409.
func (s *AuthService) linkVerifiedAccount(ctx context.Context, existing *domain.User, verified domain.VerifiedOAuthIdentity, guestSessionID *uuid.UUID) (domain.Session, bool, error) {
	if !canSignIn(existing) {
		return domain.Session{}, true, domain.ErrInvalidCredentials
	}
	if !existing.EmailVerified {
		if s.accounts == nil || existing.Role != domain.RoleCustomer || !existing.PhoneVerified {
			return domain.Session{}, true, domain.ErrOAuthAccountLinkRequired
		}
		if err := s.accounts.DetachEmail(ctx, existing.ID); err != nil {
			return domain.Session{}, true, err
		}
		return domain.Session{}, false, nil
	}
	err := s.transaction.WithinTransaction(ctx, func(_ domain.UserRepository, identities domain.OAuthIdentityRepository) error {
		_, err := identities.Create(ctx, domain.OAuthIdentity{ID: uuid.New(), UserID: existing.ID, Provider: verified.Provider, Subject: verified.Subject, ProviderEmail: normalizeContact(verified.Email), EmailVerified: verified.EmailVerified})
		return err
	})
	if errors.Is(err, domain.ErrOAuthIdentityAlreadyLinked) {
		// A concurrent callback for the same provider account linked it first.
		// That is the same outcome, if it linked to this account.
		identity, findErr := s.identities.FindByProviderSubject(ctx, verified.Provider, verified.Subject)
		if findErr != nil || identity == nil || identity.UserID != existing.ID {
			return domain.Session{}, true, domain.ErrOAuthAccountLinkRequired
		}
		err = nil
	}
	if err != nil {
		return domain.Session{}, true, err
	}
	session, err := s.finishLogin(ctx, existing, guestSessionID)
	return session, true, err
}

// Account describes the signed-in account to its own client: what it can sign
// in with, and what still needs confirming.
func (s *AuthService) Account(ctx context.Context, userID uuid.UUID) (domain.Account, error) {
	if s.users == nil || userID == uuid.Nil {
		return domain.Account{}, domain.ErrInvalidCredentials
	}
	user, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, domain.ErrUserNotFound) || (err == nil && !canSignIn(user)) {
		return domain.Account{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.Account{}, err
	}
	return domain.Account{
		UserID: user.ID, Role: user.Role, Email: user.Email, Phone: user.Phone,
		EmailVerified: user.EmailVerified, PhoneVerified: user.PhoneVerified,
		HasPassword: user.PasswordHash != "",
	}, nil
}

func (s *AuthService) finishLogin(ctx context.Context, user *domain.User, guestSessionID *uuid.UUID) (domain.Session, error) {
	if s.observer != nil {
		if err := s.observer.OnUserLogin(ctx, user.ID, guestSessionID); err != nil {
			return domain.Session{}, err
		}
	}
	return s.issue(ctx, user)
}

// issue starts a session: an access token and, where refresh tokens are
// enabled, the first refresh token of a new sign-in.
func (s *AuthService) issue(ctx context.Context, user *domain.User) (domain.Session, error) {
	session, err := s.accessSession(user)
	if err != nil || s.refreshTokens == nil {
		return session, err
	}
	session.RefreshToken, session.RefreshExpiresAt, err = s.startRefreshFamily(ctx, user.ID)
	if err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *AuthService) accessSession(user *domain.User) (domain.Session, error) {
	if user == nil || !validRole(user.Role) {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	accessToken, claims, err := s.tokens.CreateTokenForRole(user.ID, string(user.Role), s.accessTTL)
	if err != nil {
		return domain.Session{}, err
	}
	return domain.Session{AccessToken: accessToken, ExpiresAt: claims.ExpiresAt.Time, UserID: user.ID, Role: user.Role}, nil
}

func (s *AuthService) provider(code string) (domain.OAuthProvider, error) {
	if s.providers == nil {
		return nil, domain.ErrOAuthProviderUnavailable
	}
	provider, ok := s.providers.Get(strings.ToLower(strings.TrimSpace(code)))
	if !ok || provider == nil {
		return nil, domain.ErrOAuthProviderUnavailable
	}
	return provider, nil
}

// loginKey is the form a login is looked up in: a phone number as it is stored,
// anything else as typed.
func loginKey(login string) string {
	login = strings.TrimSpace(login)
	if phone, ok := normalizePhone(login); ok {
		return phone
	}
	return login
}

func normalizeContact(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	if strings.Contains(normalized, "@") {
		normalized = strings.ToLower(normalized)
	}
	return &normalized
}

func secureRandomValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate OAuth entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validRole(role domain.Role) bool {
	switch role {
	case domain.RoleCustomer, domain.RoleManager, domain.RoleAdmin, domain.RoleOwner:
		return true
	default:
		return false
	}
}

// validPassword enforces a baseline password policy. bcrypt limits inputs to
// 72 bytes, while rune counting keeps the user-facing lower bound Unicode-safe.
func validPassword(value string) bool {
	return utf8.RuneCountInString(value) >= 8 && utf8.RuneCountInString(value) <= 72 && len([]byte(value)) <= 72
}

var _ domain.AuthService = (*AuthService)(nil)

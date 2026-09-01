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
	if s.users == nil || s.tokens == nil || s.accessTTL <= 0 {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	email, phone := normalizeContact(command.Email), normalizeContact(command.Phone)
	if email == nil && phone == nil {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	if !validPassword(command.Password) {
		return domain.Session{}, domain.ErrInvalidPassword
	}
	hash, err := password.HashPassword(command.Password)
	if err != nil {
		return domain.Session{}, err
	}
	user, err := s.users.Create(ctx, domain.NewUser{Email: email, Phone: phone, PasswordHash: hash, Role: domain.RoleCustomer, Status: domain.UserStatusActive})
	if err != nil {
		return domain.Session{}, err
	}
	return s.finishLogin(ctx, user, command.GuestSessionID)
}

func (s *AuthService) LoginPassword(ctx context.Context, command domain.PasswordLoginCommand) (domain.Session, error) {
	if s.users == nil || s.tokens == nil || s.accessTTL <= 0 {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	user, err := s.users.FindByLogin(ctx, strings.TrimSpace(command.Login))
	if err != nil || user == nil || user.Status != domain.UserStatusActive || !validRole(user.Role) || user.PasswordHash == "" {
		return domain.Session{}, domain.ErrInvalidCredentials
	}
	if err := password.CheckPassword(command.Password, user.PasswordHash); err != nil {
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
		// A callback may not silently attach an OAuth identity to an existing
		// local account. A future authenticated linking use case owns that flow.
		return domain.Session{}, domain.ErrOAuthAccountLinkRequired
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

func (s *AuthService) finishLogin(ctx context.Context, user *domain.User, guestSessionID *uuid.UUID) (domain.Session, error) {
	if s.observer != nil {
		if err := s.observer.OnUserLogin(ctx, user.ID, guestSessionID); err != nil {
			return domain.Session{}, err
		}
	}
	return s.issue(user)
}

func (s *AuthService) issue(user *domain.User) (domain.Session, error) {
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

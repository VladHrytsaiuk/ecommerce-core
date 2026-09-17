package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

func TestAuthServiceCompletesVerifiedOAuthForNewUser(t *testing.T) {
	users := &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	identities := &oauthIdentityRepositoryFake{}
	attempts := &oauthAttemptStoreFake{}
	provider := &oauthProviderFake{identity: domain.VerifiedOAuthIdentity{Provider: "google", Subject: "subject-1", Email: stringPointer("buyer@example.com"), EmailVerified: true}}
	maker, err := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	if err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(users, identities, attempts, authTransactionFake{users: users, identities: identities}, NewOAuthProviderRegistry(provider), maker, time.Hour, 10*time.Minute)
	service.now = func() time.Time { return time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC) }

	authorization, err := service.BeginOAuth(context.Background(), domain.BeginOAuthCommand{Provider: "google", RedirectURI: "https://store.example.test/auth/google/callback"})
	if err != nil || authorization.RedirectURL == "" || attempts.created.State == "" {
		t.Fatalf("BeginOAuth() = (%+v, %v), attempt=%+v", authorization, err, attempts.created)
	}
	session, err := service.CompleteOAuth(context.Background(), domain.CompleteOAuthCommand{Provider: "google", RedirectURI: attempts.created.RedirectURI, Code: "provider-code", State: attempts.created.State})
	if err != nil || session.UserID == uuid.Nil || session.Role != domain.RoleCustomer || len(identities.items) != 1 {
		t.Fatalf("CompleteOAuth() = (%+v, %v), identities=%d", session, err, len(identities.items))
	}
	if provider.exchange.Nonce != attempts.created.Nonce || provider.exchange.CodeVerifier != attempts.created.CodeVerifier {
		t.Fatalf("provider exchange did not receive persisted PKCE/nonce: %+v", provider.exchange)
	}
}

func TestAuthServiceDoesNotAutoLinkExistingAccount(t *testing.T) {
	existing := &domain.User{ID: uuid.New(), Email: stringPointer("buyer@example.com"), Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	users := &userRepositoryFake{byLogin: map[string]*domain.User{"buyer@example.com": existing}, byID: map[uuid.UUID]*domain.User{existing.ID: existing}}
	identities := &oauthIdentityRepositoryFake{}
	attempts := &oauthAttemptStoreFake{created: domain.OAuthAttempt{Provider: "google", State: "state", RedirectURI: "https://store.example.test/callback", Nonce: "nonce", CodeVerifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}}
	provider := &oauthProviderFake{identity: domain.VerifiedOAuthIdentity{Provider: "google", Subject: "subject-1", Email: stringPointer("buyer@example.com"), EmailVerified: true}}
	maker, _ := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	service := NewAuthService(users, identities, attempts, authTransactionFake{users: users, identities: identities}, NewOAuthProviderRegistry(provider), maker, time.Hour, time.Minute)

	_, err := service.CompleteOAuth(context.Background(), domain.CompleteOAuthCommand{Provider: "google", RedirectURI: attempts.created.RedirectURI, Code: "provider-code", State: "state"})
	if !errors.Is(err, domain.ErrOAuthAccountLinkRequired) {
		t.Fatalf("CompleteOAuth() error = %v, want account-link requirement", err)
	}
}

func TestAuthServiceRejectsConcurrentOAuthCallbacks(t *testing.T) {
	users := &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	identities := &oauthIdentityRepositoryFake{}
	attempts := &oauthAttemptStoreFake{created: domain.OAuthAttempt{Provider: "google", State: "state", RedirectURI: "https://store.example.test/callback", Nonce: "nonce", CodeVerifier: "verifier", ExpiresAt: time.Now().Add(time.Minute)}}
	provider := &oauthProviderFake{identity: domain.VerifiedOAuthIdentity{Provider: "google", Subject: "subject-1", Email: stringPointer("buyer@example.com"), EmailVerified: true}}
	maker, _ := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	service := NewAuthService(users, identities, attempts, authTransactionFake{users: users, identities: identities}, NewOAuthProviderRegistry(provider), maker, time.Hour, time.Minute)

	start := make(chan struct{})
	errorsByCallback := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := service.CompleteOAuth(context.Background(), domain.CompleteOAuthCommand{Provider: "google", RedirectURI: attempts.created.RedirectURI, Code: "provider-code", State: "state"})
			errorsByCallback <- err
		}()
	}
	close(start)

	successes, replayFailures := 0, 0
	for range 2 {
		err := <-errorsByCallback
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrInvalidOAuthState) {
			replayFailures++
		} else {
			t.Fatalf("CompleteOAuth() error = %v", err)
		}
	}
	if successes != 1 || replayFailures != 1 {
		t.Fatalf("successes=%d replayFailures=%d, want one each", successes, replayFailures)
	}
}

func TestAuthServiceRejectsPasswordsOutsidePolicy(t *testing.T) {
	users := &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	maker, _ := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	service := NewAuthService(users, nil, nil, nil, nil, maker, time.Hour, time.Minute)
	email := "buyer@example.com"

	for _, password := range []string{"short", string(make([]byte, 73))} {
		if _, err := service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: password}); !errors.Is(err, domain.ErrInvalidPassword) {
			t.Fatalf("RegisterPassword(%d bytes) error = %v, want invalid password", len(password), err)
		}
	}
	if _, err := service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "safe-pass"}); err != nil {
		t.Fatalf("RegisterPassword() error = %v", err)
	}
}

type userRepositoryFake struct {
	byLogin map[string]*domain.User
	byID    map[uuid.UUID]*domain.User
}

func (r *userRepositoryFake) Create(_ context.Context, user domain.NewUser) (*domain.User, error) {
	if user.Email != nil {
		if _, exists := r.byLogin[*user.Email]; exists {
			return nil, domain.ErrEmailAlreadyExists
		}
	}
	created := &domain.User{ID: uuid.New(), Email: user.Email, Phone: user.Phone, PasswordHash: user.PasswordHash, Role: user.Role, Status: user.Status}
	if created.Email != nil {
		r.byLogin[*created.Email] = created
	}
	r.byID[created.ID] = created
	return created, nil
}

func (r *userRepositoryFake) FindByLogin(_ context.Context, login string) (*domain.User, error) {
	user, ok := r.byLogin[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

func (r *userRepositoryFake) FindByID(_ context.Context, userID uuid.UUID) (*domain.User, error) {
	user, ok := r.byID[userID]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

type oauthIdentityRepositoryFake struct{ items []*domain.OAuthIdentity }

func (r *oauthIdentityRepositoryFake) FindByProviderSubject(_ context.Context, provider, subject string) (*domain.OAuthIdentity, error) {
	for _, identity := range r.items {
		if identity.Provider == provider && identity.Subject == subject {
			return identity, nil
		}
	}
	return nil, domain.ErrOAuthIdentityNotFound
}

func (r *oauthIdentityRepositoryFake) Create(_ context.Context, identity domain.OAuthIdentity) (*domain.OAuthIdentity, error) {
	r.items = append(r.items, &identity)
	return &identity, nil
}

type oauthAttemptStoreFake struct {
	mu       sync.Mutex
	created  domain.OAuthAttempt
	consumed bool
}

func (r *oauthAttemptStoreFake) Create(_ context.Context, attempt domain.OAuthAttempt) error {
	r.created = attempt
	return nil
}

func (r *oauthAttemptStoreFake) Consume(_ context.Context, provider, state string, now time.Time) (*domain.OAuthAttempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.consumed || r.created.Provider != provider || r.created.State != state || !r.created.ExpiresAt.After(now) {
		return nil, domain.ErrInvalidOAuthState
	}
	r.consumed = true
	return &r.created, nil
}

type authTransactionFake struct {
	users      domain.UserRepository
	identities domain.OAuthIdentityRepository
}

func (t authTransactionFake) WithinTransaction(ctx context.Context, fn func(domain.UserRepository, domain.OAuthIdentityRepository) error) error {
	return fn(t.users, t.identities)
}

type oauthProviderFake struct {
	identity domain.VerifiedOAuthIdentity
	exchange domain.OAuthCodeExchange
}

func (*oauthProviderFake) Code() string { return "google" }

func (*oauthProviderFake) BeginAuthorization(_ context.Context, request domain.OAuthAuthorizationRequest) (domain.OAuthAuthorization, error) {
	if request.State == "" || request.Nonce == "" || request.CodeVerifier == "" {
		return domain.OAuthAuthorization{}, domain.ErrInvalidOAuthState
	}
	return domain.OAuthAuthorization{RedirectURL: "https://accounts.example.test/auth?state=" + request.State}, nil
}

func (p *oauthProviderFake) ExchangeCode(_ context.Context, request domain.OAuthCodeExchange) (domain.VerifiedOAuthIdentity, error) {
	p.exchange = request
	return p.identity, nil
}

func stringPointer(value string) *string { return &value }

// bcryptFloor is far below any real DefaultCost comparison (tens of
// milliseconds) yet far above the microseconds an early return would take, so
// the assertion separates the two paths without depending on machine speed.
const bcryptFloor = 5 * time.Millisecond

func TestLoginPasswordSpendsBcryptCostOnUnknownLogin(t *testing.T) {
	users := &userRepositoryFake{byLogin: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	maker, _ := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	service := NewAuthService(users, nil, nil, nil, nil, maker, time.Hour, time.Minute)
	passwordCostEqualizer() // Exclude one-time hash generation from the measurement.

	start := time.Now()
	_, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{
		Login: "nobody@example.com", Password: "whatever-they-typed",
	})
	elapsed := time.Since(start)

	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("LoginPassword() error = %v, want invalid credentials", err)
	}
	// Returning before bcrypt would answer an unknown address orders of
	// magnitude faster than a registered one, revealing which accounts exist.
	if elapsed < bcryptFloor {
		t.Fatalf("unknown login answered in %s, want at least %s of password-check cost", elapsed, bcryptFloor)
	}
}

func TestLoginPasswordSpendsBcryptCostOnDisabledAccount(t *testing.T) {
	email := "disabled@example.com"
	hash, err := password.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	users := &userRepositoryFake{
		byLogin: map[string]*domain.User{email: {
			ID: uuid.New(), Email: &email, PasswordHash: hash,
			Role: domain.RoleCustomer, Status: domain.UserStatusDisabled,
		}},
		byID: map[uuid.UUID]*domain.User{},
	}
	maker, _ := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	service := NewAuthService(users, nil, nil, nil, nil, maker, time.Hour, time.Minute)
	passwordCostEqualizer()

	start := time.Now()
	_, err = service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: email, Password: "correct-horse-battery"})
	elapsed := time.Since(start)

	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("LoginPassword() error = %v, want invalid credentials", err)
	}
	if elapsed < bcryptFloor {
		t.Fatalf("disabled account answered in %s, want at least %s of password-check cost", elapsed, bcryptFloor)
	}
}

func TestLoginPasswordStillAuthenticatesValidCredentials(t *testing.T) {
	email := "buyer@example.com"
	hash, err := password.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	user := &domain.User{ID: uuid.New(), Email: &email, PasswordHash: hash, Role: domain.RoleCustomer, Status: domain.UserStatusActive}
	users := &userRepositoryFake{byLogin: map[string]*domain.User{email: user}, byID: map[uuid.UUID]*domain.User{user.ID: user}}
	maker, _ := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	service := NewAuthService(users, nil, nil, nil, nil, maker, time.Hour, time.Minute)

	session, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: email, Password: "correct-horse-battery"})
	if err != nil || session.UserID != user.ID || session.AccessToken == "" {
		t.Fatalf("LoginPassword() = (%+v, %v), want a session for %s", session, err, user.ID)
	}

	if _, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: email, Password: "wrong"}); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("LoginPassword(wrong password) error = %v, want invalid credentials", err)
	}
}

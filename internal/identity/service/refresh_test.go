package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

type refreshStoreFake struct {
	created   []domain.NewRefreshToken
	rotations []domain.RotateRefreshToken
	result    domain.RotationResult
	rotateErr error
	revoked   [][]byte
	// revokedUsers are the accounts whose every sign-in was ended.
	revokedUsers []uuid.UUID
}

func (f *refreshStoreFake) Create(_ context.Context, token domain.NewRefreshToken) error {
	f.created = append(f.created, token)
	return nil
}

func (f *refreshStoreFake) Rotate(_ context.Context, request domain.RotateRefreshToken) (domain.RotationResult, error) {
	f.rotations = append(f.rotations, request)
	return f.result, f.rotateErr
}

func (f *refreshStoreFake) RevokeFamily(_ context.Context, hash []byte, _ time.Time) error {
	f.revoked = append(f.revoked, hash)
	return nil
}

func (f *refreshStoreFake) RevokeUser(_ context.Context, userID uuid.UUID, _ time.Time) error {
	f.revokedUsers = append(f.revokedUsers, userID)
	return nil
}

func (f *refreshStoreFake) PurgeExpired(context.Context, time.Time, int) (int, error) { return 0, nil }

var refreshClock = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

func refreshService(t *testing.T, users *userRepositoryFake, store *refreshStoreFake) *AuthService {
	t.Helper()
	maker, err := token.NewJWTMaker("a-secure-secret-with-at-least-thirty-two-characters")
	if err != nil {
		t.Fatal(err)
	}
	identities := &oauthIdentityRepositoryFake{}
	service := NewAuthService(users, identities, &oauthAttemptStoreFake{}, authTransactionFake{users: users, identities: identities}, NewOAuthProviderRegistry(), maker, 15*time.Minute, time.Minute)
	service.now = func() time.Time { return refreshClock }
	if store != nil {
		service.WithRefreshTokens(store, 7*24*time.Hour)
	}
	return service
}

func activeCustomer(t *testing.T) *domain.User {
	t.Helper()
	hash, err := password.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	email := "buyer@example.com"
	return &domain.User{ID: uuid.New(), Email: &email, PasswordHash: hash, Role: domain.RoleCustomer, Status: domain.UserStatusActive}
}

func usersWith(user *domain.User) *userRepositoryFake {
	return &userRepositoryFake{byLogin: map[string]*domain.User{*user.Email: user}, byID: map[uuid.UUID]*domain.User{user.ID: user}}
}

func sha(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func TestASignInReturnsARefreshTokenAndStoresOnlyItsHash(t *testing.T) {
	user := activeCustomer(t)
	store := &refreshStoreFake{}
	service := refreshService(t, usersWith(user), store)

	session, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: *user.Email, Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("LoginPassword() error = %v", err)
	}
	if session.RefreshToken == "" || len(store.created) != 1 {
		t.Fatalf("session refresh token %q, %d stored; want one issued and one stored", session.RefreshToken, len(store.created))
	}
	stored := store.created[0]
	if !bytes.Equal(stored.Hash, sha(session.RefreshToken)) || bytes.Contains(stored.Hash, []byte(session.RefreshToken)) {
		t.Fatal("the store received something other than the token's SHA-256")
	}
	if stored.UserID != user.ID || stored.FamilyID == uuid.Nil || !stored.CreatedAt.Equal(refreshClock) {
		t.Fatalf("stored = %+v, want a new family for the user created now", stored)
	}
	if want := refreshClock.Add(7 * 24 * time.Hour); !stored.ExpiresAt.Equal(want) || !session.RefreshExpiresAt.Equal(want) {
		t.Fatalf("expiry stored %s returned %s, want %s", stored.ExpiresAt, session.RefreshExpiresAt, want)
	}
}

func TestARefreshIssuesTheRoleTheUserHasNow(t *testing.T) {
	user := activeCustomer(t)
	familyExpiry := refreshClock.Add(3 * 24 * time.Hour)
	store := &refreshStoreFake{result: domain.RotationResult{Outcome: domain.RotationRotated, UserID: user.ID, ExpiresAt: familyExpiry}}
	users := usersWith(user)
	service := refreshService(t, users, store)
	// The role changed after sign-in; the refreshed token must carry the new one.
	user.Role = domain.RoleOwner

	session, err := service.RefreshSession(context.Background(), "presented-token")
	if err != nil {
		t.Fatalf("RefreshSession() error = %v", err)
	}
	if session.Role != domain.RoleOwner || session.UserID != user.ID || session.AccessToken == "" {
		t.Fatalf("session = %+v, want an access token with the current role", session)
	}
	if len(store.rotations) != 1 {
		t.Fatalf("rotations = %d, want 1", len(store.rotations))
	}
	rotation := store.rotations[0]
	if !bytes.Equal(rotation.PresentedHash, sha("presented-token")) || rotation.ReuseGrace != refreshReuseGrace || !rotation.Now.Equal(refreshClock) {
		t.Fatalf("rotation = %+v, want the presented token's hash, the grace window and now", rotation)
	}
	if session.RefreshToken == "presented-token" || !bytes.Equal(rotation.NextHash, sha(session.RefreshToken)) {
		t.Fatal("the new refresh token returned is not the one stored as the successor")
	}
	if !session.RefreshExpiresAt.Equal(familyExpiry) {
		t.Fatalf("refresh expiry = %s, want the family's %s — refreshing must not extend a sign-in", session.RefreshExpiresAt, familyExpiry)
	}
}

func TestEveryRefusedRefreshIsTheSameError(t *testing.T) {
	for name, outcome := range map[string]domain.RotationOutcome{
		"unknown": domain.RotationUnknown, "expired": domain.RotationExpired, "revoked": domain.RotationRevoked,
		"superseded": domain.RotationSuperseded, "reused": domain.RotationReused,
	} {
		t.Run(name, func(t *testing.T) {
			user := activeCustomer(t)
			store := &refreshStoreFake{result: domain.RotationResult{Outcome: outcome, UserID: user.ID}}
			_, err := refreshService(t, usersWith(user), store).RefreshSession(context.Background(), "presented-token")
			if !errors.Is(err, domain.ErrInvalidRefreshToken) {
				t.Fatalf("RefreshSession() error = %v, want ErrInvalidRefreshToken", err)
			}
		})
	}
}

func TestAnAccountThatCanNoLongerSignInLosesItsSignInOnRefresh(t *testing.T) {
	// The rotation has already stored a successor by the time the account is
	// read, so that successor's family is revoked rather than left usable.
	for name, prepare := range map[string]func(*domain.User, *userRepositoryFake){
		"disabled": func(user *domain.User, _ *userRepositoryFake) { user.Status = domain.UserStatusDisabled },
		"deleted":  func(user *domain.User, users *userRepositoryFake) { delete(users.byID, user.ID) },
	} {
		t.Run(name, func(t *testing.T) {
			user := activeCustomer(t)
			users := usersWith(user)
			store := &refreshStoreFake{result: domain.RotationResult{Outcome: domain.RotationRotated, UserID: user.ID, ExpiresAt: refreshClock.Add(time.Hour)}}
			service := refreshService(t, users, store)
			prepare(user, users)

			_, err := service.RefreshSession(context.Background(), "presented-token")

			if !errors.Is(err, domain.ErrInvalidRefreshToken) {
				t.Fatalf("RefreshSession() error = %v, want ErrInvalidRefreshToken", err)
			}
			if len(store.revoked) != 1 || !bytes.Equal(store.revoked[0], store.rotations[0].NextHash) {
				t.Fatal("the successor's family was not revoked")
			}
		})
	}
}

func TestLogoutRevokesTheFamilyAndIgnoresWhatIsNotAToken(t *testing.T) {
	store := &refreshStoreFake{}
	service := refreshService(t, usersWith(activeCustomer(t)), store)

	if err := service.RevokeSession(context.Background(), "a-real-looking-token"); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if len(store.revoked) != 1 || !bytes.Equal(store.revoked[0], sha("a-real-looking-token")) {
		t.Fatal("logout did not revoke the presented token's family")
	}
	for _, junk := range []string{"", "   ", string(bytes.Repeat([]byte("x"), maxRefreshTokenLength+1))} {
		if err := service.RevokeSession(context.Background(), junk); err != nil {
			t.Fatalf("RevokeSession(%q) error = %v, want nil", junk, err)
		}
	}
	if len(store.revoked) != 1 {
		t.Fatalf("revocations = %d, want junk ignored without touching the store", len(store.revoked))
	}
}

func TestWithoutRefreshTokensSessionsCarryNone(t *testing.T) {
	user := activeCustomer(t)
	service := refreshService(t, usersWith(user), nil)

	session, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: *user.Email, Password: "correct-horse-battery"})
	if err != nil || session.RefreshToken != "" {
		t.Fatalf("LoginPassword() = (%+v, %v), want a session with no refresh token", session, err)
	}
	if _, err := service.RefreshSession(context.Background(), "anything"); !errors.Is(err, domain.ErrInvalidRefreshToken) {
		t.Fatalf("RefreshSession() error = %v, want ErrInvalidRefreshToken", err)
	}
}

func TestADisabledPasswordMethodCannotBeReachedThroughTheService(t *testing.T) {
	// The routes are left out when password sign-in is off; this is what stops
	// any other caller from reaching it anyway.
	user := activeCustomer(t)
	service := refreshService(t, usersWith(user), nil).WithPasswordSignIn(false)

	if _, err := service.LoginPassword(context.Background(), domain.PasswordLoginCommand{Login: *user.Email, Password: "correct-horse-battery"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("LoginPassword() error = %v, want ErrSignInMethodDisabled", err)
	}
	email := "new@example.com"
	if _, err := service.RegisterPassword(context.Background(), domain.RegisterPasswordCommand{Email: &email, Password: "correct-horse-battery"}); !errors.Is(err, domain.ErrSignInMethodDisabled) {
		t.Fatalf("RegisterPassword() error = %v, want ErrSignInMethodDisabled", err)
	}
}

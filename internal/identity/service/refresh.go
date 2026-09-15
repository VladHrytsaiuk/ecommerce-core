package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

const (
	refreshTokenBytes = 32
	// maxRefreshTokenLength bounds what is hashed. A real token is 43
	// characters; anything much longer is not one.
	maxRefreshTokenLength = 128
	// refreshReuseGrace is how soon after its use a token may be presented
	// again without being treated as stolen. Two browser tabs refreshing
	// together present the same token within moments; without a grace window
	// the second would revoke the sign-in the first had just renewed. The cost
	// is that theft is recognised only once the copy is presented later than
	// this.
	refreshReuseGrace = 30 * time.Second
)

// WithRefreshTokens enables refresh tokens: every sign-in then also returns one,
// valid for ttl from the moment of sign-in. Without it sessions end with their
// access token, as they did before.
func (s *AuthService) WithRefreshTokens(store domain.RefreshTokenStore, ttl time.Duration) *AuthService {
	if s != nil && store != nil && ttl > 0 {
		s.refreshTokens, s.refreshTTL = store, ttl
	}
	return s
}

// RefreshSession exchanges a refresh token for a new access token and a new
// refresh token.
//
// The access token carries the user's role as it is now, read from the
// database, so a demotion takes effect at the next refresh rather than at the
// end of the sign-in.
func (s *AuthService) RefreshSession(ctx context.Context, presented string) (domain.Session, error) {
	presented = strings.TrimSpace(presented)
	if s.refreshTokens == nil || s.users == nil || s.tokens == nil || s.accessTTL <= 0 || presented == "" || len(presented) > maxRefreshTokenLength {
		return domain.Session{}, domain.ErrInvalidRefreshToken
	}
	nextToken, nextHash, err := newRefreshToken()
	if err != nil {
		return domain.Session{}, err
	}
	now := s.now().UTC()
	result, err := s.refreshTokens.Rotate(ctx, domain.RotateRefreshToken{
		PresentedHash: hashRefreshToken(presented),
		NextID:        uuid.New(),
		NextHash:      nextHash,
		Now:           now,
		ReuseGrace:    refreshReuseGrace,
	})
	if err != nil {
		return domain.Session{}, err
	}
	if result.Outcome != domain.RotationRotated {
		return domain.Session{}, domain.ErrInvalidRefreshToken
	}

	user, err := s.users.FindByID(ctx, result.UserID)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return domain.Session{}, err
	}
	if err != nil || user == nil || user.Status != domain.UserStatusActive || !validRole(user.Role) {
		// A disabled or deleted account keeps no sign-in. The successor was
		// already stored by the rotation, so the family is revoked outright.
		if revokeErr := s.refreshTokens.RevokeFamily(ctx, nextHash, now); revokeErr != nil {
			return domain.Session{}, revokeErr
		}
		return domain.Session{}, domain.ErrInvalidRefreshToken
	}

	session, err := s.accessSession(user)
	if err != nil {
		return domain.Session{}, err
	}
	session.RefreshToken, session.RefreshExpiresAt = nextToken, result.ExpiresAt
	return session, nil
}

// RevokeSession ends the sign-in a refresh token belongs to. A token that is
// empty, malformed or unknown is not an error, so logging out reveals nothing
// about the token presented.
func (s *AuthService) RevokeSession(ctx context.Context, presented string) error {
	presented = strings.TrimSpace(presented)
	if s.refreshTokens == nil || presented == "" || len(presented) > maxRefreshTokenLength {
		return nil
	}
	return s.refreshTokens.RevokeFamily(ctx, hashRefreshToken(presented), s.now().UTC())
}

// startRefreshFamily issues the first refresh token of a new sign-in.
func (s *AuthService) startRefreshFamily(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	token, hash, err := newRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.refreshTTL)
	if err := s.refreshTokens.Create(ctx, domain.NewRefreshToken{
		ID: uuid.New(), UserID: userID, FamilyID: uuid.New(),
		Hash: hash, ExpiresAt: expiresAt, CreatedAt: now,
	}); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func newRefreshToken() (string, []byte, error) {
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashRefreshToken(token), nil
}

func hashRefreshToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

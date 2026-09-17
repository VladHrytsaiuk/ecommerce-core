package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidRefreshToken covers every refresh token that cannot be exchanged:
// unknown, expired, revoked, already used. One error for all of them, so the
// endpoint says nothing about which.
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

// RotationOutcome is what presenting a refresh token led to.
type RotationOutcome int

const (
	// RotationUnknown: no token has this hash.
	RotationUnknown RotationOutcome = iota
	// RotationRotated: the token was consumed and its successor stored.
	RotationRotated
	// RotationExpired: the sign-in the token belongs to has ended.
	RotationExpired
	// RotationRevoked: the family was revoked, by logout or by a detected reuse.
	RotationRevoked
	// RotationSuperseded: the token was used moments ago — two browser tabs
	// refreshing together. Refused, but not treated as theft.
	RotationSuperseded
	// RotationReused: a consumed token was presented again after the grace
	// window, so two parties hold it. The whole family has been revoked.
	RotationReused
)

// NewRefreshToken is a token to store. Only the SHA-256 of the value the client
// holds is kept, so the table cannot be replayed if it is read.
type NewRefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	Hash      []byte
	ExpiresAt time.Time
	CreatedAt time.Time
}

// RotateRefreshToken consumes PresentedHash and, if it may be exchanged, stores
// NextHash as its successor in the same family and with the same expiry.
type RotateRefreshToken struct {
	PresentedHash []byte
	NextID        uuid.UUID
	NextHash      []byte
	Now           time.Time
	ReuseGrace    time.Duration
}

// RotationResult names the user and family behind the presented token whenever
// it was found, so a caller can act on them even when the exchange was refused.
type RotationResult struct {
	Outcome   RotationOutcome
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	ExpiresAt time.Time
}

// RefreshTokenStore persists refresh token families.
type RefreshTokenStore interface {
	Create(ctx context.Context, token NewRefreshToken) error
	// Rotate decides and applies the outcome in one transaction. A detected
	// reuse revokes the family as part of that transaction, which is why it is
	// an outcome rather than an error: an error would roll the revocation back.
	Rotate(ctx context.Context, request RotateRefreshToken) (RotationResult, error)
	// RevokeFamily ends the sign-in the presented token belongs to. An unknown
	// token is not an error.
	RevokeFamily(ctx context.Context, presentedHash []byte, now time.Time) error
	// RevokeUser ends every sign-in of an account. It joins a transaction
	// carried in ctx.
	RevokeUser(ctx context.Context, userID uuid.UUID, now time.Time) error
	PurgeExpired(ctx context.Context, now time.Time, limit int) (int, error)
}

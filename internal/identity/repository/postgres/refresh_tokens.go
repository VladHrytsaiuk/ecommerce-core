package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// RefreshTokenStore keeps refresh token families in refresh_tokens.
type RefreshTokenStore struct{ db *gorm.DB }

func NewRefreshTokenStore(db *gorm.DB) *RefreshTokenStore { return &RefreshTokenStore{db: db} }

func (s *RefreshTokenStore) Create(ctx context.Context, token domain.NewRefreshToken) error {
	if s == nil || s.db == nil || len(token.Hash) != sha256.Size || token.ID == uuid.Nil || token.UserID == uuid.Nil || token.FamilyID == uuid.Nil || !token.ExpiresAt.After(token.CreatedAt) {
		return fmt.Errorf("invalid refresh token")
	}
	return s.db.WithContext(ctx).Exec(`
INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, token.ID, token.UserID, token.FamilyID, token.Hash, token.ExpiresAt, token.CreatedAt).Error
}

type refreshTokenRow struct {
	ID        uuid.UUID  `gorm:"column:id"`
	UserID    uuid.UUID  `gorm:"column:user_id"`
	FamilyID  uuid.UUID  `gorm:"column:family_id"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

// Rotate decides the outcome and applies it in one transaction.
//
// Every statement that changes a family runs under that family's advisory lock.
// A row lock on the presented token is not enough: revoking a family updates
// the rows that exist when the UPDATE starts, and a concurrent rotation of
// another token in the same family could commit a successor just after —
// leaving a live token in a family that had just been revoked for theft.
func (s *RefreshTokenStore) Rotate(ctx context.Context, request domain.RotateRefreshToken) (domain.RotationResult, error) {
	if s == nil || s.db == nil || len(request.PresentedHash) != sha256.Size || len(request.NextHash) != sha256.Size || request.NextID == uuid.Nil || request.Now.IsZero() || request.ReuseGrace < 0 {
		return domain.RotationResult{}, fmt.Errorf("invalid refresh token rotation")
	}
	var result domain.RotationResult
	err := transaction.Within(ctx, s.db, func(tx *gorm.DB) error {
		row, found, err := lockFamilyAndToken(tx, request.PresentedHash)
		if err != nil || !found {
			return err
		}
		result = domain.RotationResult{UserID: row.UserID, FamilyID: row.FamilyID, ExpiresAt: row.ExpiresAt}
		switch {
		case row.RevokedAt != nil:
			result.Outcome = domain.RotationRevoked
		case !row.ExpiresAt.After(request.Now):
			result.Outcome = domain.RotationExpired
		case row.UsedAt != nil && request.Now.Sub(*row.UsedAt) <= request.ReuseGrace:
			result.Outcome = domain.RotationSuperseded
		case row.UsedAt != nil:
			if err := revokeFamily(tx, row.FamilyID, request.Now); err != nil {
				return err
			}
			result.Outcome = domain.RotationReused
		default:
			if err := tx.Exec(`UPDATE refresh_tokens SET used_at = ? WHERE id = ?`, request.Now, row.ID).Error; err != nil {
				return err
			}
			// The successor inherits the family's expiry: refreshing keeps a
			// sign-in alive but never extends it.
			if err := tx.Exec(`
INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, request.NextID, row.UserID, row.FamilyID, request.NextHash, row.ExpiresAt, request.Now).Error; err != nil {
				return err
			}
			result.Outcome = domain.RotationRotated
		}
		return nil
	})
	if err != nil {
		return domain.RotationResult{}, err
	}
	return result, nil
}

func (s *RefreshTokenStore) RevokeFamily(ctx context.Context, presentedHash []byte, now time.Time) error {
	if s == nil || s.db == nil || len(presentedHash) != sha256.Size || now.IsZero() {
		return fmt.Errorf("invalid refresh token revocation")
	}
	return transaction.Within(ctx, s.db, func(tx *gorm.DB) error {
		row, found, err := lockFamilyAndToken(tx, presentedHash)
		if err != nil || !found {
			return err
		}
		return revokeFamily(tx, row.FamilyID, now)
	})
}

// PurgeExpired removes tokens whose sign-in has ended. A used or revoked token
// is kept until then on purpose: it is what recognises a replay, and every
// token in a family shares one expiry, so nothing is purged while any member
// of its family could still be presented.
func (s *RefreshTokenStore) PurgeExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	if s == nil || s.db == nil || now.IsZero() || limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("invalid refresh token purge request")
	}
	result := s.db.WithContext(ctx).Exec(`
DELETE FROM refresh_tokens
WHERE id IN (
    SELECT id FROM refresh_tokens
    WHERE expires_at <= ?
    ORDER BY expires_at
    LIMIT ?
)`, now, limit)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// lockFamilyAndToken takes the family's advisory lock, then the token's row
// lock. The family is looked up first without a lock because the advisory lock
// is keyed by it; the row is then re-read under both locks, so a decision is
// never made on a row another transaction was still changing.
func lockFamilyAndToken(tx *gorm.DB, hash []byte) (refreshTokenRow, bool, error) {
	var family struct {
		FamilyID uuid.UUID `gorm:"column:family_id"`
	}
	lookup := tx.Raw(`SELECT family_id FROM refresh_tokens WHERE token_hash = ?`, hash).Scan(&family)
	if lookup.Error != nil {
		return refreshTokenRow{}, false, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		return refreshTokenRow{}, false, nil
	}
	if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`, family.FamilyID.String()).Error; err != nil {
		return refreshTokenRow{}, false, err
	}
	var row refreshTokenRow
	locked := tx.Raw(`
SELECT id, user_id, family_id, expires_at, used_at, revoked_at
FROM refresh_tokens
WHERE token_hash = ?
FOR UPDATE`, hash).Scan(&row)
	if locked.Error != nil {
		return refreshTokenRow{}, false, locked.Error
	}
	return row, locked.RowsAffected == 1, nil
}

func revokeFamily(tx *gorm.DB, familyID uuid.UUID, now time.Time) error {
	return tx.Exec(`UPDATE refresh_tokens SET revoked_at = ? WHERE family_id = ? AND revoked_at IS NULL`, now, familyID).Error
}

var _ domain.RefreshTokenStore = (*RefreshTokenStore)(nil)

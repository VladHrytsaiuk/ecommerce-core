package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// CodeStore keeps one-time codes in one_time_codes.
//
// Everything that reads or changes an address's codes runs under that
// address's advisory lock. Counting the codes sent in the last hour and then
// inserting one is a check-then-act: without the lock, parallel requests would
// all count below the limit and all send.
type CodeStore struct{ db *gorm.DB }

func NewCodeStore(db *gorm.DB) *CodeStore { return &CodeStore{db: db} }

const (
	codeHourWindow = time.Hour
	codeDayWindow  = 24 * time.Hour
)

func (s *CodeStore) Issue(ctx context.Context, request domain.IssueCode, within func(context.Context) error) error {
	limits := request.Limits
	if s == nil || s.db == nil || within == nil || request.ID == uuid.Nil || !validChannel(request.Channel) || !validPurpose(request.Purpose) ||
		len(request.DestinationHash) != sha256.Size || len(request.CodeHash) != sha256.Size ||
		request.Now.IsZero() || !request.ExpiresAt.After(request.Now) ||
		limits.Cooldown < 0 || limits.PerHour < 1 || limits.PerDay < limits.PerHour || limits.MaxAttempts < 1 {
		return fmt.Errorf("invalid one-time code")
	}
	return transaction.Within(ctx, s.db, func(tx *gorm.DB) error {
		if err := lockDestination(tx, request.Channel, request.DestinationHash); err != nil {
			return err
		}
		if !request.Unthrottled {
			retryAfter, err := throttle(tx, request)
			if err != nil {
				return err
			}
			if retryAfter > 0 {
				return &domain.CodeThrottledError{RetryAfter: retryAfter}
			}
		}
		if err := tx.Exec(`
UPDATE one_time_codes SET closed_at = ?
WHERE channel = ? AND destination_hash = ? AND purpose = ? AND closed_at IS NULL`,
			request.Now, string(request.Channel), request.DestinationHash, string(request.Purpose)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`
INSERT INTO one_time_codes (id, channel, purpose, destination_hash, code_hash, max_attempts, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			request.ID, string(request.Channel), string(request.Purpose), request.DestinationHash, request.CodeHash,
			limits.MaxAttempts, request.ExpiresAt, request.Now).Error; err != nil {
			return err
		}
		return within(transaction.WithContext(ctx, tx))
	})
}

// throttle returns how long the address must wait before it may be sent
// another code, or zero. Codes for every purpose count: each is a message to
// the same inbox. Where several limits apply, the longest wait wins.
func throttle(tx *gorm.DB, request domain.IssueCode) (time.Duration, error) {
	var window struct {
		Latest     *time.Time `gorm:"column:latest"`
		HourCount  int        `gorm:"column:hour_count"`
		HourOldest *time.Time `gorm:"column:hour_oldest"`
		DayCount   int        `gorm:"column:day_count"`
		DayOldest  *time.Time `gorm:"column:day_oldest"`
	}
	hourStart, dayStart := request.Now.Add(-codeHourWindow), request.Now.Add(-codeDayWindow)
	if err := tx.Raw(`
SELECT max(created_at) AS latest,
       count(*) FILTER (WHERE created_at > ?) AS hour_count,
       min(created_at) FILTER (WHERE created_at > ?) AS hour_oldest,
       count(*) AS day_count,
       min(created_at) AS day_oldest
FROM one_time_codes
WHERE channel = ? AND destination_hash = ? AND created_at > ?`,
		hourStart, hourStart, string(request.Channel), request.DestinationHash, dayStart).Scan(&window).Error; err != nil {
		return 0, err
	}
	var wait time.Duration
	longer := func(until time.Time) {
		if remaining := until.Sub(request.Now); remaining > wait {
			wait = remaining
		}
	}
	if window.Latest != nil {
		longer(window.Latest.Add(request.Limits.Cooldown))
	}
	if window.HourCount >= request.Limits.PerHour && window.HourOldest != nil {
		longer(window.HourOldest.Add(codeHourWindow))
	}
	if window.DayCount >= request.Limits.PerDay && window.DayOldest != nil {
		longer(window.DayOldest.Add(codeDayWindow))
	}
	return wait, nil
}

func (s *CodeStore) Consume(ctx context.Context, request domain.ConsumeCode, onAccepted func(context.Context) error) (bool, error) {
	if s == nil || s.db == nil || onAccepted == nil || !validChannel(request.Channel) || !validPurpose(request.Purpose) ||
		len(request.DestinationHash) != sha256.Size || len(request.CodeHash) != sha256.Size || request.Now.IsZero() {
		return false, fmt.Errorf("invalid one-time code verification")
	}
	accepted := false
	err := transaction.Within(ctx, s.db, func(tx *gorm.DB) error {
		if err := lockDestination(tx, request.Channel, request.DestinationHash); err != nil {
			return err
		}
		var code struct {
			ID       uuid.UUID `gorm:"column:id"`
			CodeHash []byte    `gorm:"column:code_hash"`
		}
		found := tx.Raw(`
SELECT id, code_hash
FROM one_time_codes
WHERE channel = ? AND destination_hash = ? AND purpose = ? AND closed_at IS NULL AND expires_at > ?
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE`, string(request.Channel), request.DestinationHash, string(request.Purpose), request.Now).Scan(&code)
		if found.Error != nil || found.RowsAffected == 0 {
			return found.Error
		}
		if subtle.ConstantTimeCompare(code.CodeHash, request.CodeHash) != 1 {
			// The attempt that exhausts the code also closes it, so the count of
			// guesses a code allows is exactly max_attempts.
			return tx.Exec(`
UPDATE one_time_codes
SET attempts = attempts + 1,
    closed_at = CASE WHEN attempts + 1 >= max_attempts THEN ? ELSE closed_at END
WHERE id = ?`, request.Now, code.ID).Error
		}
		if err := tx.Exec(`UPDATE one_time_codes SET closed_at = ? WHERE id = ?`, request.Now, code.ID).Error; err != nil {
			return err
		}
		if err := onAccepted(transaction.WithContext(ctx, tx)); err != nil {
			return err
		}
		accepted = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return accepted, nil
}

// PurgeSettled removes codes issued more than a day ago. A code is useless long
// before that, but the per-address limits are counted from the last day's
// codes, and a purge that removed them sooner would reset the daily limit.
func (s *CodeStore) PurgeSettled(ctx context.Context, now time.Time, limit int) (int, error) {
	if s == nil || s.db == nil || now.IsZero() || limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("invalid one-time code purge request")
	}
	result := s.db.WithContext(ctx).Exec(`
DELETE FROM one_time_codes
WHERE id IN (
    SELECT id FROM one_time_codes
    WHERE created_at < ?
    ORDER BY created_at
    LIMIT ?
)`, now.Add(-codeDayWindow), limit)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func lockDestination(tx *gorm.DB, channel domain.CodeChannel, destinationHash []byte) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`,
		"one_time_code:"+string(channel)+":"+hex.EncodeToString(destinationHash)).Error
}

func validChannel(channel domain.CodeChannel) bool {
	return channel == domain.CodeChannelEmail
}

func validPurpose(purpose domain.CodePurpose) bool {
	switch purpose {
	case domain.CodePurposeSignIn, domain.CodePurposeVerifyEmail, domain.CodePurposeResetPassword:
		return true
	default:
		return false
	}
}

// CodeAccounts reads and changes the accounts a code proves an address for. It
// joins the transaction carried in ctx.
type CodeAccounts struct{ db *gorm.DB }

func NewCodeAccounts(db *gorm.DB) *CodeAccounts { return &CodeAccounts{db: db} }

func (a *CodeAccounts) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if a == nil || a.db == nil || email == "" {
		return nil, fmt.Errorf("invalid account lookup")
	}
	var user *domain.User
	err := transaction.Within(ctx, a.db, func(tx *gorm.DB) error {
		var err error
		user, err = findByEmail(tx, email)
		return err
	})
	return user, err
}

func (a *CodeAccounts) FindOrCreateByEmail(ctx context.Context, email string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if a == nil || a.db == nil || email == "" {
		return nil, fmt.Errorf("invalid account lookup")
	}
	var user *domain.User
	err := transaction.Within(ctx, a.db, func(tx *gorm.DB) error {
		// ON CONFLICT rather than find-then-create: a unique violation aborts the
		// whole transaction, and a Google sign-up for the same address may commit
		// between the two. The conflict target is users_email_unique.
		if err := tx.Exec(`
INSERT INTO users (id, email, email_verified, role, status)
VALUES (?, ?, TRUE, ?, ?)
ON CONFLICT (lower(email)) WHERE email IS NOT NULL DO NOTHING`,
			uuid.New(), email, string(domain.RoleCustomer), string(domain.UserStatusActive)).Error; err != nil {
			return err
		}
		var err error
		user, err = findByEmail(tx, email)
		return err
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (a *CodeAccounts) ClaimEmail(ctx context.Context, userID uuid.UUID) error {
	if a == nil || a.db == nil || userID == uuid.Nil {
		return fmt.Errorf("invalid email claim")
	}
	return transaction.Within(ctx, a.db, func(tx *gorm.DB) error {
		return tx.Exec(`
UPDATE users
SET email_verified = TRUE, password_hash = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, userID).Error
	})
}

func (a *CodeAccounts) VerifyEmail(ctx context.Context, userID uuid.UUID, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if a == nil || a.db == nil || userID == uuid.Nil || email == "" {
		return false, fmt.Errorf("invalid email verification")
	}
	verified := false
	err := transaction.Within(ctx, a.db, func(tx *gorm.DB) error {
		result := tx.Exec(`
UPDATE users
SET email_verified = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND lower(email) = ?`, userID, email)
		verified = result.RowsAffected == 1
		return result.Error
	})
	return verified, err
}

func (a *CodeAccounts) SetPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	if a == nil || a.db == nil || userID == uuid.Nil || passwordHash == "" {
		return fmt.Errorf("invalid password change")
	}
	return transaction.Within(ctx, a.db, func(tx *gorm.DB) error {
		result := tx.Exec(`
UPDATE users
SET password_hash = ?, email_verified = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, passwordHash, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrUserNotFound
		}
		return nil
	})
}

func findByEmail(tx *gorm.DB, email string) (*domain.User, error) {
	var record userRecord
	if err := tx.Where("lower(email) = ?", email).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return record.toDomain(), nil
}

var _ domain.CodeStore = (*CodeStore)(nil)
var _ domain.CodeAccounts = (*CodeAccounts)(nil)

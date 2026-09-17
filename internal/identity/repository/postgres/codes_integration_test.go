//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// testCodeLimits are small so the limits can be reached in a few calls.
var testCodeLimits = domain.CodeLimits{Cooldown: time.Minute, PerHour: 3, PerDay: 4, MaxAttempts: 3}

func issueCode(store *CodeStore, destination, code string, now time.Time, deliver func(context.Context) error) error {
	if deliver == nil {
		deliver = func(context.Context) error { return nil }
	}
	return store.Issue(context.Background(), domain.IssueCode{
		ID: uuid.New(), Channel: domain.CodeChannelEmail, Purpose: domain.CodePurposeSignIn,
		DestinationHash: tokenHash(destination), CodeHash: tokenHash(destination + ":" + code),
		Now: now, ExpiresAt: now.Add(10 * time.Minute), Limits: testCodeLimits,
	}, deliver)
}

func consumeCode(t *testing.T, store *CodeStore, destination, code string, now time.Time, onAccepted func(context.Context) error) (bool, error) {
	t.Helper()
	if onAccepted == nil {
		onAccepted = func(context.Context) error { return nil }
	}
	return store.Consume(context.Background(), domain.ConsumeCode{
		Channel: domain.CodeChannelEmail, Purpose: domain.CodePurposeSignIn, DestinationHash: tokenHash(destination),
		CodeHash: tokenHash(destination + ":" + code), Now: now,
	}, onAccepted)
}

func mustIssue(t *testing.T, store *CodeStore, destination, code string, now time.Time) {
	t.Helper()
	if err := issueCode(store, destination, code, now, nil); err != nil {
		t.Fatalf("Issue(%s at %s) error = %v", destination, now, err)
	}
}

func throttledFor(t *testing.T, err error) time.Duration {
	t.Helper()
	var throttled *domain.CodeThrottledError
	if !errors.As(err, &throttled) {
		t.Fatalf("error = %v, want a throttle", err)
	}
	return throttled.RetryAfter
}

// The rules are about SQL — the advisory lock, attempts that commit alongside a
// refusal, a rollback that leaves a code usable — so none can be shown against
// a fake.
func TestOneTimeCodes(t *testing.T) {
	_, db := newAttemptStore(t)
	store := NewCodeStore(db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("a code is stored with the message queued in the same transaction", func(t *testing.T) {
		destination := "issue-" + uuid.NewString()
		err := issueCode(store, destination, "111111", now, func(ctx context.Context) error {
			tx, err := transaction.FromContext(ctx)
			if err != nil {
				return err
			}
			return tx.Exec(`SELECT 1`).Error
		})
		if err != nil {
			t.Fatalf("Issue() error = %v, want deliver to find the transaction in its context", err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM one_time_codes WHERE destination_hash = ? AND closed_at IS NULL AND attempts = 0 AND max_attempts = 3`, tokenHash(destination)) != 1 {
			t.Fatal("the code was not stored open with its attempt budget")
		}
	})

	t.Run("a failed delivery stores no code", func(t *testing.T) {
		destination := "undelivered-" + uuid.NewString()
		if err := issueCode(store, destination, "111111", now, func(context.Context) error { return errors.New("queue down") }); err == nil {
			t.Fatal("Issue() succeeded although delivery failed")
		}
		if countRows(t, db, `SELECT COUNT(*) FROM one_time_codes WHERE destination_hash = ?`, tokenHash(destination)) != 0 {
			t.Fatal("a code nobody was sent was stored")
		}
		// And it does not count towards the cooldown.
		mustIssue(t, store, destination, "222222", now)
	})

	t.Run("an address waits a minute between codes, and a new code replaces the old", func(t *testing.T) {
		destination := "cooldown-" + uuid.NewString()
		mustIssue(t, store, destination, "111111", now)

		if wait := throttledFor(t, issueCode(store, destination, "222222", now.Add(20*time.Second), nil)); wait != 40*time.Second {
			t.Fatalf("RetryAfter = %s, want the 40s left of the cooldown", wait)
		}
		mustIssue(t, store, destination, "333333", now.Add(time.Minute))

		if accepted, err := consumeCode(t, store, destination, "111111", now.Add(61*time.Second), nil); err != nil || accepted {
			t.Fatalf("Consume(replaced code) = (%v, %v), want refused", accepted, err)
		}
		if accepted, err := consumeCode(t, store, destination, "333333", now.Add(61*time.Second), nil); err != nil || !accepted {
			t.Fatalf("Consume(newest code) = (%v, %v), want accepted", accepted, err)
		}
	})

	t.Run("the hourly and daily limits hold, and say how long to wait", func(t *testing.T) {
		destination := "limits-" + uuid.NewString()
		for i := range testCodeLimits.PerHour {
			mustIssue(t, store, destination, "111111", now.Add(time.Duration(i)*time.Minute))
		}
		// Three codes at 0, 1 and 2 minutes: the fourth waits for the first to
		// leave the hour.
		if wait := throttledFor(t, issueCode(store, destination, "111111", now.Add(10*time.Minute), nil)); wait != 50*time.Minute {
			t.Fatalf("hourly RetryAfter = %s, want 50m", wait)
		}
		mustIssue(t, store, destination, "111111", now.Add(time.Hour+time.Second))
		// Four in the day: the fifth waits for the first to leave the day.
		later := now.Add(3 * time.Hour)
		if wait := throttledFor(t, issueCode(store, destination, "111111", later, nil)); wait != 21*time.Hour {
			t.Fatalf("daily RetryAfter = %s, want 21h", wait)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM one_time_codes WHERE destination_hash = ?`, tokenHash(destination)) != 4 {
			t.Fatal("a throttled request stored a code")
		}
	})

	t.Run("wrong guesses are counted even though they are refused, and use the code up", func(t *testing.T) {
		destination := "attempts-" + uuid.NewString()
		mustIssue(t, store, destination, "123456", now)

		for i := 1; i <= testCodeLimits.MaxAttempts; i++ {
			if accepted, err := consumeCode(t, store, destination, "000000", now.Add(time.Second), nil); err != nil || accepted {
				t.Fatalf("guess %d = (%v, %v), want refused without error", i, accepted, err)
			}
			if got := countRows(t, db, `SELECT attempts FROM one_time_codes WHERE destination_hash = ?`, tokenHash(destination)); got != int64(i) {
				t.Fatalf("after guess %d attempts = %d; a refusal must not roll its count back", i, got)
			}
		}
		if accepted, err := consumeCode(t, store, destination, "123456", now.Add(time.Second), nil); err != nil || accepted {
			t.Fatalf("the right code after the attempts ran out = (%v, %v), want refused", accepted, err)
		}
	})

	t.Run("a replaced code stays closed when the newer one is used up", func(t *testing.T) {
		// Otherwise exhausting the newest code would reopen the one before it,
		// and every request would add a fresh budget of guesses at old codes.
		destination := "replaced-" + uuid.NewString()
		mustIssue(t, store, destination, "111111", now)
		mustIssue(t, store, destination, "222222", now.Add(time.Minute))
		for range testCodeLimits.MaxAttempts {
			if _, err := consumeCode(t, store, destination, "000000", now.Add(61*time.Second), nil); err != nil {
				t.Fatal(err)
			}
		}
		if accepted, err := consumeCode(t, store, destination, "111111", now.Add(62*time.Second), nil); err != nil || accepted {
			t.Fatalf("Consume(replaced code) = (%v, %v), want refused", accepted, err)
		}
	})

	t.Run("purposes are kept apart, but share the address's limits", func(t *testing.T) {
		destination := "purposes-" + uuid.NewString()
		issue := func(purpose domain.CodePurpose, code string, at time.Time, unthrottled bool) error {
			return store.Issue(context.Background(), domain.IssueCode{
				ID: uuid.New(), Channel: domain.CodeChannelEmail, Purpose: purpose,
				DestinationHash: tokenHash(destination), CodeHash: tokenHash(destination + ":" + code),
				Now: at, ExpiresAt: at.Add(10 * time.Minute), Limits: testCodeLimits, Unthrottled: unthrottled,
			}, func(context.Context) error { return nil })
		}
		consume := func(purpose domain.CodePurpose, code string, at time.Time) bool {
			accepted, err := store.Consume(context.Background(), domain.ConsumeCode{
				Channel: domain.CodeChannelEmail, Purpose: purpose,
				DestinationHash: tokenHash(destination), CodeHash: tokenHash(destination + ":" + code), Now: at,
			}, func(context.Context) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			return accepted
		}

		if err := issue(domain.CodePurposeVerifyEmail, "111111", now, false); err != nil {
			t.Fatal(err)
		}
		// A different purpose is still inside the address's cooldown...
		throttledFor(t, issue(domain.CodePurposeResetPassword, "222222", now.Add(time.Second), false))
		// ...unless it is the code registration sends.
		if err := issue(domain.CodePurposeResetPassword, "222222", now.Add(2*time.Second), true); err != nil {
			t.Fatalf("Issue(unthrottled) = %v", err)
		}
		// A new code of one purpose does not replace the other's, and neither is
		// accepted for the other purpose.
		if consume(domain.CodePurposeResetPassword, "111111", now.Add(3*time.Second)) {
			t.Fatal("a verification code was accepted to reset a password")
		}
		if !consume(domain.CodePurposeVerifyEmail, "111111", now.Add(4*time.Second)) {
			t.Fatal("issuing a reset code closed the verification code")
		}
		if !consume(domain.CodePurposeResetPassword, "222222", now.Add(5*time.Second)) {
			t.Fatal("the reset code was not accepted for its purpose")
		}
	})

	t.Run("texts stop at the store-wide hourly cap, whatever the number", func(t *testing.T) {
		// Its own clock, so codes from other subtests do not count.
		at := now.Add(30 * 24 * time.Hour)
		limits := testCodeLimits
		limits.ChannelPerHour = 2
		issuePhone := func(number string, when time.Time) error {
			return store.Issue(context.Background(), domain.IssueCode{
				ID: uuid.New(), Channel: domain.CodeChannelPhone, Purpose: domain.CodePurposeSignIn,
				DestinationHash: tokenHash(number), CodeHash: tokenHash(number + ":123456"),
				Now: when, ExpiresAt: when.Add(10 * time.Minute), Limits: limits,
			}, func(context.Context) error { return nil })
		}
		prefix := "+38050" + uuid.NewString()[:4]
		if err := issuePhone(prefix+"1", at); err != nil {
			t.Fatal(err)
		}
		if err := issuePhone(prefix+"2", at.Add(10*time.Minute)); err != nil {
			t.Fatal(err)
		}
		// A third number, never texted before, still waits for the first text
		// to leave the hour.
		if wait := throttledFor(t, issuePhone(prefix+"3", at.Add(20*time.Minute))); wait != 40*time.Minute {
			t.Fatalf("RetryAfter = %s, want the 40m until the oldest text leaves the hour", wait)
		}
		// Email is another channel, and uncapped.
		if err := store.Issue(context.Background(), domain.IssueCode{
			ID: uuid.New(), Channel: domain.CodeChannelEmail, Purpose: domain.CodePurposeSignIn,
			DestinationHash: tokenHash(prefix + "@example.test"), CodeHash: tokenHash("x"),
			Now: at.Add(20 * time.Minute), ExpiresAt: at.Add(30 * time.Minute), Limits: limits,
		}, func(context.Context) error { return nil }); err != nil {
			t.Fatalf("Issue(email) under the phone cap = %v", err)
		}
	})

	t.Run("an expired code is refused", func(t *testing.T) {
		destination := "expired-" + uuid.NewString()
		mustIssue(t, store, destination, "123456", now)
		if accepted, err := consumeCode(t, store, destination, "123456", now.Add(10*time.Minute), nil); err != nil || accepted {
			t.Fatalf("Consume(expired) = (%v, %v), want refused", accepted, err)
		}
	})

	t.Run("a code works once, and not if what it signs in to fails", func(t *testing.T) {
		destination := "once-" + uuid.NewString()
		mustIssue(t, store, destination, "123456", now)

		accepted, err := consumeCode(t, store, destination, "123456", now.Add(time.Second), func(context.Context) error { return errors.New("account unavailable") })
		if err == nil || accepted {
			t.Fatalf("Consume() with a failing sign-in = (%v, %v), want the error", accepted, err)
		}
		accepted, err = consumeCode(t, store, destination, "123456", now.Add(2*time.Second), func(ctx context.Context) error {
			_, err := transaction.FromContext(ctx)
			return err
		})
		if err != nil || !accepted {
			t.Fatalf("Consume() after the rollback = (%v, %v), want the code still usable", accepted, err)
		}
		if accepted, err := consumeCode(t, store, destination, "123456", now.Add(3*time.Second), nil); err != nil || accepted {
			t.Fatalf("Consume() a second time = (%v, %v), want refused", accepted, err)
		}
	})

	t.Run("a request waits for one in progress, so the cooldown cannot be raced", func(t *testing.T) {
		// Stand in for a request that holds the address's lock and is about to
		// store its code.
		destination := "race-" + uuid.NewString()
		inProgress := db.Begin()
		if inProgress.Error != nil {
			t.Fatal(inProgress.Error)
		}
		defer inProgress.Rollback()
		if err := lockDestination(inProgress, domain.CodeChannelEmail, tokenHash(destination)); err != nil {
			t.Fatal(err)
		}

		second := make(chan error, 1)
		go func() { second <- issueCode(store, destination, "222222", now, nil) }()
		// A scheduling margin, as in the refresh token lock test: without the
		// lock the second request counts no codes and stores its own inside it.
		time.Sleep(300 * time.Millisecond)

		if err := inProgress.Exec(`INSERT INTO one_time_codes (id, channel, purpose, destination_hash, code_hash, max_attempts, expires_at, created_at) VALUES (?, 'email', 'sign_in', ?, ?, 3, ?, ?)`,
			uuid.New(), tokenHash(destination), tokenHash("first"), now.Add(10*time.Minute), now).Error; err != nil {
			t.Fatal(err)
		}
		if err := inProgress.Commit().Error; err != nil {
			t.Fatal(err)
		}
		if wait := throttledFor(t, <-second); wait != time.Minute {
			t.Fatalf("RetryAfter = %s, want the full cooldown", wait)
		}
	})

	t.Run("addresses are locked apart", func(t *testing.T) {
		held := db.Begin()
		defer held.Rollback()
		if err := lockDestination(held, domain.CodeChannelEmail, tokenHash("locked-"+uuid.NewString())); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- issueCode(store, "unlocked-"+uuid.NewString(), "111111", now, nil) }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a request for one address waited on another address's lock")
		}
	})

	t.Run("the purge keeps a day of codes, because the limits are counted from them", func(t *testing.T) {
		destination := "purge-" + uuid.NewString()
		old, recent := now.Add(-25*time.Hour), now.Add(-23*time.Hour)
		for _, createdAt := range []time.Time{old, recent} {
			if err := db.Exec(`INSERT INTO one_time_codes (id, channel, purpose, destination_hash, code_hash, max_attempts, expires_at, created_at, closed_at) VALUES (?, 'email', 'sign_in', ?, ?, 3, ?, ?, ?)`,
				uuid.New(), tokenHash(destination), tokenHash(createdAt.String()), createdAt.Add(10*time.Minute), createdAt, createdAt.Add(time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
		}
		for {
			removed, err := store.PurgeSettled(context.Background(), now, 10)
			if err != nil {
				t.Fatal(err)
			}
			if removed == 0 {
				break
			}
		}
		if countRows(t, db, `SELECT COUNT(*) FROM one_time_codes WHERE destination_hash = ? AND created_at = ?`, tokenHash(destination), recent) != 1 ||
			countRows(t, db, `SELECT COUNT(*) FROM one_time_codes WHERE destination_hash = ?`, tokenHash(destination)) != 1 {
			t.Fatal("the purge did not remove exactly the code older than a day")
		}
	})
}

func TestCodeAccounts(t *testing.T) {
	_, db := newAttemptStore(t)
	accounts := NewCodeAccounts(db)
	ctx := context.Background()

	t.Run("a new address becomes a verified customer account", func(t *testing.T) {
		email := "new-" + uuid.NewString() + "@example.test"
		user, err := accounts.FindOrCreateByEmail(ctx, email)
		if err != nil {
			t.Fatal(err)
		}
		if user.ID == uuid.Nil || user.Email == nil || *user.Email != email || !user.EmailVerified || user.Role != domain.RoleCustomer || user.Status != domain.UserStatusActive || user.PasswordHash != "" {
			t.Fatalf("created = %+v, want an active verified customer without a password", user)
		}
	})

	t.Run("an existing address is found, whatever its case, and left as it was", func(t *testing.T) {
		id, email := uuid.New(), "Existing-"+uuid.NewString()+"@Example.test"
		if err := db.Exec(`INSERT INTO users (id, email, password_hash, role, status) VALUES (?, ?, 'hash', 'manager', 'active')`, id, email).Error; err != nil {
			t.Fatal(err)
		}
		user, err := accounts.FindOrCreateByEmail(ctx, email)
		if err != nil {
			t.Fatal(err)
		}
		if user.ID != id || user.EmailVerified || user.PasswordHash != "hash" || user.Role != domain.RoleManager {
			t.Fatalf("found = %+v, want the existing unverified manager untouched", user)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE lower(email) = lower(?)`, email) != 1 {
			t.Fatal("a second account was created for an existing address")
		}
	})

	t.Run("inside a transaction that rolls back, no account is left behind", func(t *testing.T) {
		email := "rolled-back-" + uuid.NewString() + "@example.test"
		err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
			if _, err := accounts.FindOrCreateByEmail(transaction.WithContext(ctx, tx), email); err != nil {
				return err
			}
			return errors.New("sign-in failed later")
		})
		if err == nil {
			t.Fatal("the transaction did not fail")
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE email = ?`, email) != 0 {
			t.Fatal("the account was created outside the caller's transaction")
		}
	})

	t.Run("an address is found whatever its case, and a missing one is reported", func(t *testing.T) {
		id, email := uuid.New(), "Find-"+uuid.NewString()+"@Example.test"
		if err := db.Exec(`INSERT INTO users (id, email, role, status) VALUES (?, ?, 'customer', 'active')`, id, email).Error; err != nil {
			t.Fatal(err)
		}
		if user, err := accounts.FindByEmail(ctx, email); err != nil || user.ID != id {
			t.Fatalf("FindByEmail() = (%+v, %v), want the account", user, err)
		}
		if _, err := accounts.FindByEmail(ctx, "missing-"+uuid.NewString()+"@example.test"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("FindByEmail(missing) = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("verifying an address holds only while the account still has it", func(t *testing.T) {
		id, email := uuid.New(), "verify-"+uuid.NewString()+"@example.test"
		if err := db.Exec(`INSERT INTO users (id, email, password_hash, role, status) VALUES (?, ?, 'hash', 'customer', 'active')`, id, email).Error; err != nil {
			t.Fatal(err)
		}
		if verified, err := accounts.VerifyEmail(ctx, id, "changed-"+email); err != nil || verified {
			t.Fatalf("VerifyEmail(another address) = (%v, %v), want not verified", verified, err)
		}
		if verified, err := accounts.VerifyEmail(ctx, id, email); err != nil || !verified {
			t.Fatalf("VerifyEmail() = (%v, %v), want verified", verified, err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ? AND email_verified AND password_hash = 'hash'`, id) != 1 {
			t.Fatal("verifying the address did not mark it, or touched the password")
		}
	})

	t.Run("setting a password also verifies the address it was reset from", func(t *testing.T) {
		id := uuid.New()
		if err := db.Exec(`INSERT INTO users (id, email, password_hash, role, status) VALUES (?, ?, 'old', 'customer', 'active')`, id, "reset-"+id.String()+"@example.test").Error; err != nil {
			t.Fatal(err)
		}
		if err := accounts.SetPassword(ctx, id, "new"); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ? AND email_verified AND password_hash = 'new'`, id) != 1 {
			t.Fatal("the password was not replaced, or the address not verified")
		}
		if err := accounts.SetPassword(ctx, uuid.New(), "new"); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("SetPassword(no account) = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("an account created inside a transaction goes with it", func(t *testing.T) {
		users := NewUserRepository(db)
		email := "registered-" + uuid.NewString() + "@example.test"
		err := transaction.Within(ctx, db, func(tx *gorm.DB) error {
			if _, err := users.Create(transaction.WithContext(ctx, tx), domain.NewUser{Email: &email, PasswordHash: "hash", Role: domain.RoleCustomer, Status: domain.UserStatusActive}); err != nil {
				return err
			}
			return errors.New("the code could not be queued")
		})
		if err == nil {
			t.Fatal("the transaction did not fail")
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE email = ?`, email) != 0 {
			t.Fatal("registration created the account outside the transaction that stores its code")
		}
	})

	t.Run("a new number becomes a customer account with the number verified", func(t *testing.T) {
		phone := "+38067" + fmt.Sprintf("%07d", time.Now().UnixNano()%10000000)
		user, err := accounts.FindOrCreateByPhone(ctx, phone)
		if err != nil {
			t.Fatal(err)
		}
		if user.Phone == nil || *user.Phone != phone || !user.PhoneVerified || user.EmailVerified || user.Email != nil || user.Role != domain.RoleCustomer {
			t.Fatalf("created = %+v, want a customer with only a verified phone", user)
		}
		again, err := accounts.FindOrCreateByPhone(ctx, phone)
		if err != nil || again.ID != user.ID {
			t.Fatalf("FindOrCreateByPhone() again = (%+v, %v), want the same account", again, err)
		}
	})

	t.Run("claiming a number verifies it and removes the password", func(t *testing.T) {
		id := uuid.New()
		phone := "+38063" + fmt.Sprintf("%07d", time.Now().UnixNano()%10000000)
		if err := db.Exec(`INSERT INTO users (id, phone, password_hash, role, status) VALUES (?, ?, 'squatter', 'customer', 'active')`, id, phone).Error; err != nil {
			t.Fatal(err)
		}
		if err := accounts.ClaimPhone(ctx, id); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ? AND phone_verified AND password_hash IS NULL`, id) != 1 {
			t.Fatal("the claimed account kept its password or its number stayed unverified")
		}
	})

	t.Run("claiming an address verifies it and removes the password", func(t *testing.T) {
		id := uuid.New()
		if err := db.Exec(`INSERT INTO users (id, email, password_hash, role, status) VALUES (?, ?, 'squatter', 'customer', 'active')`, id, "claim-"+id.String()+"@example.test").Error; err != nil {
			t.Fatal(err)
		}
		if err := accounts.ClaimEmail(ctx, id); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM users WHERE id = ? AND email_verified AND password_hash IS NULL`, id) != 1 {
			t.Fatal("the claimed account kept its password or stayed unverified")
		}
	})
}

func TestRevokingAUserEndsEverySignInOfThatUserOnly(t *testing.T) {
	_, db := newAttemptStore(t)
	store := NewRefreshTokenStore(db)
	now := time.Now().UTC().Truncate(time.Microsecond)
	ctx := context.Background()

	t.Run("every family of the user, and nobody else's", func(t *testing.T) {
		user, bystander := seedRefreshUser(t, db), seedRefreshUser(t, db)
		startFamily(t, store, user, "user-a-"+user.String(), now, now.Add(time.Hour))
		startFamily(t, store, user, "user-b-"+user.String(), now, now.Add(time.Hour))
		startFamily(t, store, bystander, "bystander-"+bystander.String(), now, now.Add(time.Hour))

		if err := store.RevokeUser(ctx, user, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ? AND revoked_at IS NULL`, user) != 0 {
			t.Fatal("a sign-in of the user survived")
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ? AND revoked_at IS NULL`, bystander) != 1 {
			t.Fatal("another user's sign-in was revoked")
		}
	})

	t.Run("a successor stored by a rotation in progress is caught", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "claim-a-"+user.String(), now, now.Add(time.Hour))

		rotation := db.Begin()
		if rotation.Error != nil {
			t.Fatal(rotation.Error)
		}
		defer rotation.Rollback()
		if err := rotation.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`, family.String()).Error; err != nil {
			t.Fatal(err)
		}
		revoked := make(chan error, 1)
		go func() { revoked <- store.RevokeUser(ctx, user, now.Add(time.Minute)) }()
		time.Sleep(300 * time.Millisecond)

		if err := rotation.Exec(`INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New(), user, family, tokenHash("claim-b-"+user.String()), now.Add(time.Hour), now.Add(30*time.Second)).Error; err != nil {
			t.Fatal(err)
		}
		if err := rotation.Commit().Error; err != nil {
			t.Fatal(err)
		}
		if err := <-revoked; err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id = ? AND revoked_at IS NULL`, user) != 0 {
			t.Fatal("a successor stored during the revocation survived it")
		}
	})
}

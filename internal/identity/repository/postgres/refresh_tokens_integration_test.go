//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/identity/domain"
)

func tokenHash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func seedRefreshUser(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, email, role, status) VALUES (?, ?, 'customer', 'active')`, id, "refresh-"+id.String()+"@example.test").Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func startFamily(t *testing.T, store *RefreshTokenStore, userID uuid.UUID, token string, createdAt, expiresAt time.Time) uuid.UUID {
	t.Helper()
	family := uuid.New()
	if err := store.Create(context.Background(), domain.NewRefreshToken{ID: uuid.New(), UserID: userID, FamilyID: family, Hash: tokenHash(token), ExpiresAt: expiresAt, CreatedAt: createdAt}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return family
}

func rotate(t *testing.T, store *RefreshTokenStore, presented, next string, now time.Time) domain.RotationResult {
	t.Helper()
	result, err := store.Rotate(context.Background(), domain.RotateRefreshToken{PresentedHash: tokenHash(presented), NextID: uuid.New(), NextHash: tokenHash(next), Now: now, ReuseGrace: 30 * time.Second})
	if err != nil {
		t.Fatalf("Rotate(%s) error = %v", presented, err)
	}
	return result
}

func countRows(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := db.Raw(query, args...).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

// One container for every rule. The rules are about SQL — row and advisory
// locks, the family expiry, a revocation that must survive the refusal it
// accompanies — so none of them can be shown against a fake.
func TestRefreshTokenFamilies(t *testing.T) {
	_, db := newAttemptStore(t)
	store := NewRefreshTokenStore(db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("a rotation consumes the token and its successor keeps the family's expiry", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "rotate-a", now, now.Add(time.Hour))

		result := rotate(t, store, "rotate-a", "rotate-b", now.Add(time.Minute))

		if result.Outcome != domain.RotationRotated || result.UserID != user || result.FamilyID != family || !result.ExpiresAt.Equal(now.Add(time.Hour)) {
			t.Fatalf("Rotate() = %+v, want rotated within the family and its expiry", result)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = ? AND used_at IS NOT NULL`, tokenHash("rotate-a")) != 1 {
			t.Fatal("the presented token was not marked used")
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE family_id = ? AND expires_at = ?`, family, now.Add(time.Hour)) != 2 {
			t.Fatal("the successor does not share the family's expiry; refreshing would extend the sign-in")
		}
	})

	t.Run("a replay within the grace window is refused without ending the sign-in", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "tabs-a", now, now.Add(time.Hour))
		rotate(t, store, "tabs-a", "tabs-b", now.Add(time.Minute))

		if result := rotate(t, store, "tabs-a", "tabs-c", now.Add(time.Minute+10*time.Second)); result.Outcome != domain.RotationSuperseded {
			t.Fatalf("Rotate() = %+v, want superseded", result)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE family_id = ? AND revoked_at IS NOT NULL`, family) != 0 {
			t.Fatal("a second tab ended the sign-in the first had just renewed")
		}
		if result := rotate(t, store, "tabs-b", "tabs-d", now.Add(2*time.Minute)); result.Outcome != domain.RotationRotated {
			t.Fatalf("the successor stopped working: %+v", result)
		}
	})

	t.Run("a replay after the grace window revokes the whole family, and the revocation stays", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "theft-a", now, now.Add(time.Hour))
		rotate(t, store, "theft-a", "theft-b", now.Add(time.Minute))

		if result := rotate(t, store, "theft-a", "theft-c", now.Add(5*time.Minute)); result.Outcome != domain.RotationReused {
			t.Fatalf("Rotate() = %+v, want reused", result)
		}
		// Read through a fresh statement: the revocation was committed even
		// though the rotation itself was refused.
		if revoked := countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE family_id = ? AND revoked_at IS NOT NULL`, family); revoked != 2 {
			t.Fatalf("%d of 2 tokens revoked, want the whole family", revoked)
		}
		if result := rotate(t, store, "theft-b", "theft-d", now.Add(6*time.Minute)); result.Outcome != domain.RotationRevoked {
			t.Fatalf("the thief's successor still works: %+v", result)
		}
	})

	t.Run("an expired or unknown token is refused", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		startFamily(t, store, user, "expired-a", now.Add(-2*time.Hour), now.Add(-time.Hour))
		if result := rotate(t, store, "expired-a", "expired-b", now); result.Outcome != domain.RotationExpired {
			t.Fatalf("Rotate() = %+v, want expired", result)
		}
		if result := rotate(t, store, "never-issued", "unknown-b", now); result.Outcome != domain.RotationUnknown {
			t.Fatalf("Rotate() = %+v, want unknown", result)
		}
	})

	t.Run("logout revokes the family, and an unknown token is not an error", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "logout-a", now, now.Add(time.Hour))
		rotate(t, store, "logout-a", "logout-b", now.Add(time.Minute))

		if err := store.RevokeFamily(context.Background(), tokenHash("logout-b"), now.Add(2*time.Minute)); err != nil {
			t.Fatalf("RevokeFamily() error = %v", err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE family_id = ? AND revoked_at IS NULL`, family) != 0 {
			t.Fatal("a token of the signed-out family is still live")
		}
		if err := store.RevokeFamily(context.Background(), tokenHash("never-issued-logout"), now); err != nil {
			t.Fatalf("RevokeFamily(unknown) error = %v, want nil", err)
		}
	})

	t.Run("two refreshes of one token at the same moment: exactly one succeeds", func(t *testing.T) {
		// A check of the outcome, not of the locking. The two calls usually
		// reach the database one after the other, so this passes even with every
		// lock removed; a mutation test showed exactly that. What proves the
		// family lock is the next subtest, which controls the interleaving.
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "race-a", now, now.Add(time.Hour))

		var wg sync.WaitGroup
		outcomes := make(chan domain.RotationOutcome, 2)
		start := make(chan struct{})
		for _, next := range []string{"race-b", "race-c"} {
			wg.Add(1)
			go func(next string) {
				defer wg.Done()
				<-start
				result, err := store.Rotate(context.Background(), domain.RotateRefreshToken{PresentedHash: tokenHash("race-a"), NextID: uuid.New(), NextHash: tokenHash(next), Now: now.Add(time.Minute), ReuseGrace: 30 * time.Second})
				if err != nil {
					t.Errorf("Rotate() error = %v", err)
					return
				}
				outcomes <- result.Outcome
			}(next)
		}
		close(start)
		wg.Wait()
		close(outcomes)

		counts := map[domain.RotationOutcome]int{}
		for outcome := range outcomes {
			counts[outcome]++
		}
		if counts[domain.RotationRotated] != 1 || counts[domain.RotationSuperseded] != 1 {
			t.Fatalf("outcomes = %v, want one rotated and one superseded", counts)
		}
		if rows := countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE family_id = ?`, family); rows != 2 {
			t.Fatalf("family has %d tokens, want the original and exactly one successor", rows)
		}
	})

	t.Run("a theft revocation also catches a successor stored while it waited", func(t *testing.T) {
		// What the family's advisory lock is for, and what the same-token race
		// above cannot show: a row lock on the replayed token does not stop a
		// concurrent rotation of a different token in the family from storing a
		// successor that the revocation's UPDATE never sees.
		user := seedRefreshUser(t, db)
		family := startFamily(t, store, user, "lock-a", now, now.Add(time.Hour))
		rotate(t, store, "lock-a", "lock-b", now.Add(time.Minute))

		// Stand in for a rotation of lock-b that holds the family lock and is
		// about to store its successor.
		rotation := db.Begin()
		if rotation.Error != nil {
			t.Fatal(rotation.Error)
		}
		defer rotation.Rollback()
		if err := rotation.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?::text, 0))`, family.String()).Error; err != nil {
			t.Fatal(err)
		}

		replay := make(chan domain.RotationResult, 1)
		go func() {
			result, err := store.Rotate(context.Background(), domain.RotateRefreshToken{PresentedHash: tokenHash("lock-a"), NextID: uuid.New(), NextHash: tokenHash("lock-x"), Now: now.Add(10 * time.Minute), ReuseGrace: 30 * time.Second})
			if err != nil {
				t.Errorf("Rotate() error = %v", err)
			}
			replay <- result
		}()
		// A scheduling margin. Without the lock the replay finishes inside it and
		// the successor below is stored after the revocation, which fails this
		// test. With the lock the replay waits for the commit however long this
		// is. A machine too slow to even start the replay within the margin could
		// let the lock-free version pass — never the other way round.
		time.Sleep(300 * time.Millisecond)

		if err := rotation.Exec(`UPDATE refresh_tokens SET used_at = ? WHERE token_hash = ?`, now.Add(10*time.Minute), tokenHash("lock-b")).Error; err != nil {
			t.Fatal(err)
		}
		if err := rotation.Exec(`INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New(), user, family, tokenHash("lock-c"), now.Add(time.Hour), now.Add(10*time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		if err := rotation.Commit().Error; err != nil {
			t.Fatal(err)
		}

		if result := <-replay; result.Outcome != domain.RotationReused {
			t.Fatalf("Rotate() = %+v, want reused", result)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = ? AND revoked_at IS NOT NULL`, tokenHash("lock-c")) != 1 {
			t.Fatal("a successor stored while the theft was being revoked survived the revocation")
		}
	})

	t.Run("the purge removes only sign-ins that have ended", func(t *testing.T) {
		user := seedRefreshUser(t, db)
		startFamily(t, store, user, "purge-ended", now.Add(-3*time.Hour), now.Add(-2*time.Hour))
		startFamily(t, store, user, "purge-live", now, now.Add(time.Hour))
		ended := countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE expires_at <= ?`, now)

		removed, err := store.PurgeExpired(context.Background(), now, 1000)
		if err != nil {
			t.Fatalf("PurgeExpired() error = %v", err)
		}
		if int64(removed) != ended {
			t.Fatalf("removed %d, want the %d ended tokens", removed, ended)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM refresh_tokens WHERE token_hash = ?`, tokenHash("purge-live")) != 1 {
			t.Fatal("the purge removed a live sign-in")
		}
	})
}

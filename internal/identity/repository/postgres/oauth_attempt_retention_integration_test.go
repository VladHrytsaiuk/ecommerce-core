//go:build integration

package postgres

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// oauth_authorization_attempts is ephemeral by contract — an attempt is valid
// for OAUTH_ATTEMPT_TTL and useless after — and nothing ever deleted a row.
// One per "sign in with Google" click accumulated forever, each holding that
// exchange's PKCE code_verifier and OIDC nonce in the clear.

func TestPurgeSettledRemovesConsumedAndExpiredAttempts(t *testing.T) {
	store, db := newAttemptStore(t)
	seedAttempts(t, db, "consumed", 12)
	seedAttempts(t, db, "expired", 8)
	seedAttempts(t, db, "live", 5)

	removed, err := store.PurgeSettled(context.Background(), time.Now().UTC(), 1000)
	if err != nil {
		t.Fatalf("PurgeSettled() error = %v", err)
	}
	if removed != 20 {
		t.Fatalf("PurgeSettled() removed %d, want the 12 consumed and 8 expired", removed)
	}

	var remaining int64
	if err := db.Raw(`SELECT COUNT(*) FROM oauth_authorization_attempts`).Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 5 {
		t.Fatalf("%d attempts left, want the 5 still open", remaining)
	}
}

func TestPurgeSettledNeverTouchesAnAttemptStillInFlight(t *testing.T) {
	// A sweep that deleted a live attempt would break the sign-in it belongs
	// to: the callback arrives seconds later and the state no longer resolves.
	store, db := newAttemptStore(t)
	seedAttempts(t, db, "live", 6)

	removed, err := store.PurgeSettled(context.Background(), time.Now().UTC(), 1000)
	if err != nil {
		t.Fatalf("PurgeSettled() error = %v", err)
	}
	if removed != 0 {
		t.Fatalf("PurgeSettled() removed %d open attempts, want none", removed)
	}
}

func TestPurgeSettledStopsAtItsBatchSize(t *testing.T) {
	store, db := newAttemptStore(t)
	seedAttempts(t, db, "consumed", 30)

	removed, err := store.PurgeSettled(context.Background(), time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("PurgeSettled() error = %v", err)
	}
	if removed != 10 {
		t.Fatalf("PurgeSettled() removed %d, want exactly the batch size", removed)
	}
}

func TestPurgeSettledRefusesAnUnboundedRequest(t *testing.T) {
	store, _ := newAttemptStore(t)
	for name, limit := range map[string]int{"no limit": 0, "past the ceiling": 10001} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.PurgeSettled(context.Background(), time.Now().UTC(), limit); err == nil {
				t.Fatal("PurgeSettled() accepted an unbounded request")
			}
		})
	}
}

// seedAttempts writes attempts in one of three states: already exchanged,
// past its window, or still open.
func seedAttempts(t *testing.T, db *gorm.DB, state string, rows int) {
	t.Helper()
	expiresAt, consumedAt := "CURRENT_TIMESTAMP + INTERVAL '10 minutes'", "NULL"
	switch state {
	case "consumed":
		consumedAt = "CURRENT_TIMESTAMP"
	case "expired":
		expiresAt = "CURRENT_TIMESTAMP - INTERVAL '1 hour'"
	case "live":
	default:
		t.Fatalf("unknown attempt state %q", state)
	}
	if err := db.Exec(`
INSERT INTO oauth_authorization_attempts (id, provider, state_hash, redirect_uri, nonce, code_verifier, expires_at, consumed_at)
SELECT gen_random_uuid(), 'google', digest(gen_random_uuid()::text || $1, 'sha256'),
       'https://store.example/callback', 'nonce-' || n, 'verifier-' || n,
       `+expiresAt+`, `+consumedAt+`
FROM generate_series(1, $2) AS n`, state, rows).Error; err != nil {
		t.Fatal(err)
	}
}

func newAttemptStore(t *testing.T) (*OAuthAttemptStore, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("identity_retention"),
		containerPostgres.WithUsername("identity"),
		containerPostgres.WithPassword("identity"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
	runner, err := migrate.New("file://"+filepath.Join(root, "migrations", "core"), attemptMigrationURL(dsn))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = runner.Close() }()
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return NewOAuthAttemptStore(db), db
}

func attemptMigrationURL(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", "schema_migrations")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

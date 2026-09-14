//go:build integration

package events

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

// Retention used to mean "move a completed delivery from event_deliveries to
// event_delivery_archive". Nothing emptied the archive — there was no DELETE
// against it anywhere in the codebase — so the database grew exactly as fast
// as it would have with no retention at all.

func TestPruneArchiveRemovesHistoryPastItsWindow(t *testing.T) {
	store, db := newRetentionStore(t)
	seedArchive(t, db, 40, 120*24*time.Hour) // old enough to go
	seedArchive(t, db, 10, 24*time.Hour)     // still inside the window

	removed, err := store.PruneArchive(context.Background(), time.Now().UTC().Add(-90*24*time.Hour), 1000)
	if err != nil {
		t.Fatalf("PruneArchive() error = %v", err)
	}
	if removed != 40 {
		t.Fatalf("PruneArchive() removed %d rows, want the 40 past the window", removed)
	}

	var remaining int64
	if err := db.Raw(`SELECT COUNT(*) FROM event_delivery_archive`).Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 10 {
		t.Fatalf("%d rows left, want the 10 still inside the window", remaining)
	}
}

func TestPruneArchiveStopsAtItsBatchSize(t *testing.T) {
	// An unbounded DELETE holds its connection for as long as the backlog
	// takes. Every other retention statement in this core is batched.
	store, db := newRetentionStore(t)
	seedArchive(t, db, 25, 120*24*time.Hour)

	removed, err := store.PruneArchive(context.Background(), time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("PruneArchive() error = %v", err)
	}
	if removed != 10 {
		t.Fatalf("PruneArchive() removed %d rows, want exactly the batch size", removed)
	}
}

func TestPruneArchiveRefusesAnUnboundedRequest(t *testing.T) {
	store, _ := newRetentionStore(t)
	for name, limit := range map[string]int{"no limit": 0, "past the ceiling": 10001} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.PruneArchive(context.Background(), time.Now().UTC(), limit); err == nil {
				t.Fatal("PruneArchive() accepted an unbounded request")
			}
		})
	}
}

func seedArchive(t *testing.T, db *gorm.DB, rows int, age time.Duration) {
	t.Helper()
	if err := db.Exec(`
INSERT INTO event_delivery_archive (event_id, consumer, status, attempts, completed_at, archived_at)
SELECT gen_random_uuid(), 'notifications', 'done', 1,
       CURRENT_TIMESTAMP - ?::interval, CURRENT_TIMESTAMP
FROM generate_series(1, ?)`, age.String(), rows).Error; err != nil {
		t.Fatal(err)
	}
}

func newRetentionStore(t *testing.T) (*RetentionStore, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("outbox_retention"),
		containerPostgres.WithUsername("outbox"),
		containerPostgres.WithPassword("outbox"),
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
	runner, err := migrate.New("file://"+filepath.Join(root, "migrations", "core"), retentionMigrationURL(dsn))
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
	return NewRetentionStore(db), db
}

func retentionMigrationURL(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", "schema_migrations")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

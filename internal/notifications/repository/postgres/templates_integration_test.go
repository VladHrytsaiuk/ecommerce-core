//go:build integration

package postgres

import (
	"context"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	containerPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	notifications "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
)

// The table ships empty. Whether the defaults actually land in it, and whether
// a redeploy leaves a store's own wording alone, are both properties of this
// SQL and nothing above it can show them.

func TestTheDefaultsLandForTheStoresLocale(t *testing.T) {
	repository, db := newTemplateRepository(t)

	if err := repository.SynchronizeTemplates(context.Background(), "UK", notifications.DefaultTemplates); err != nil {
		t.Fatalf("SynchronizeTemplates() error = %v", err)
	}

	var stored int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_templates WHERE locale = 'uk' AND is_active`).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != int64(len(notifications.DefaultTemplates)) {
		t.Fatalf("stored %d templates, want %d", stored, len(notifications.DefaultTemplates))
	}
	// The locale is normalised, or FindTemplate's lookup would miss it.
	found, err := repository.FindTemplate(context.Background(), notifications.OrderPaidTemplate, "uk", "uk")
	if err != nil || found == nil {
		t.Fatalf("FindTemplate() = (%v, %v), want the seeded template", found, err)
	}
}

func TestSynchronizeIsIdempotent(t *testing.T) {
	// It runs on every boot.
	repository, db := newTemplateRepository(t)
	ctx := context.Background()

	for range 3 {
		if err := repository.SynchronizeTemplates(ctx, "en", notifications.DefaultTemplates); err != nil {
			t.Fatalf("SynchronizeTemplates() error = %v", err)
		}
	}
	var stored int64
	if err := db.Raw(`SELECT COUNT(*) FROM notification_templates`).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != int64(len(notifications.DefaultTemplates)) {
		t.Fatalf("three runs left %d rows, want %d", stored, len(notifications.DefaultTemplates))
	}
}

func TestAStoresOwnWordingSurvivesADeploy(t *testing.T) {
	// This is why it is DO NOTHING and not an upsert: an operator who rewrote
	// the order receipt must not find this core's default back in place.
	repository, db := newTemplateRepository(t)
	ctx := context.Background()
	if err := repository.SynchronizeTemplates(ctx, "en", notifications.DefaultTemplates); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE notification_templates SET subject_template = 'Дякуємо за замовлення {{.OrderNumber}}'
		WHERE template_key = ? AND locale = 'en'`, notifications.OrderPaidTemplate).Error; err != nil {
		t.Fatal(err)
	}

	if err := repository.SynchronizeTemplates(ctx, "en", notifications.DefaultTemplates); err != nil {
		t.Fatal(err)
	}

	found, err := repository.FindTemplate(ctx, notifications.OrderPaidTemplate, "en", "en")
	if err != nil || found == nil {
		t.Fatal(err)
	}
	if found.Subject != "Дякуємо за замовлення {{.OrderNumber}}" {
		t.Fatalf("subject = %q; the deploy overwrote the store's own wording", found.Subject)
	}
}

func TestEveryDefaultIsReachableThroughTheFallbackLocale(t *testing.T) {
	// A customer browsing in a locale with no templates of its own must still
	// get mail, which is the whole point of the fallback.
	repository, _ := newTemplateRepository(t)
	ctx := context.Background()
	if err := repository.SynchronizeTemplates(ctx, "en", notifications.DefaultTemplates); err != nil {
		t.Fatal(err)
	}

	for _, template := range notifications.DefaultTemplates {
		found, err := repository.FindTemplate(ctx, template.Key, "de", "en")
		if err != nil {
			t.Fatalf("FindTemplate(%q) error = %v", template.Key, err)
		}
		if found == nil {
			t.Fatalf("no template for %q through the fallback; its mail would die after ten attempts", template.Key)
		}
	}
}

func TestAnEmptyLocaleIsRefused(t *testing.T) {
	repository, _ := newTemplateRepository(t)
	if err := repository.SynchronizeTemplates(context.Background(), "  ", notifications.DefaultTemplates); err == nil {
		t.Fatal("SynchronizeTemplates() accepted a blank locale")
	}
}

func newTemplateRepository(t *testing.T) (*Repository, *gorm.DB) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("notifications_test"),
		containerPostgres.WithUsername("notifications"),
		containerPostgres.WithPassword("notifications"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
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
	for _, step := range []struct{ dir, table string }{
		{filepath.Join(root, "migrations", "core"), "schema_migrations"},
		{filepath.Join(root, "migrations", "modules", "notifications"), "schema_migrations_module_notifications"},
	} {
		if err := migrateTemplateDir(step.dir, dsn, step.table); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(db), db
}

func migrateTemplateDir(dir, dsn, table string) error {
	m, err := migrate.New("file://"+dir, templateMigrationURL(dsn, table))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func templateMigrationURL(dsn, table string) string {
	parsed, err := url.Parse(dsn)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

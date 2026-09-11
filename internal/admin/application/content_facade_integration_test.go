//go:build integration

package application_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	postgresContainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	adminApplication "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/application"
	adminPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/repository/postgres"
	badgesDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/domain"
	badgesPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/badges/repository/postgres"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	eventsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/events"
	seoDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/domain"
	seoPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/seo/repository/postgres"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/cache"
)

// These tests exist because the unit tests cannot prove what matters here. A
// fake transaction that stamps the context proves the facade passes txCtx down;
// it says nothing about whether the repository underneath uses it. Only a real
// PostgreSQL transaction shows whether a write joined it, so these run against
// one and assert on the committed table contents.

// TestContentFacadeCreateBadgeReadsBackInsideItsOwnTransaction fails when a
// repository reads through the pool after writing through the transaction: the
// row is not committed yet, so a second connection cannot see it and the read
// back reports ErrNotFound.
func TestContentFacadeCreateBadgeReadsBackInsideItsOwnTransaction(t *testing.T) {
	db, actorID := newContentFacadeFixture(t)
	facade := newContentFacade(t, db, actorID, eventsPostgres.NewPublisher(eventsDomain.ConsumerAdminAudit))

	created, err := facade.CreateBadge(context.Background(), adminApplication.CatalogCommand{ActorUserID: actorID}, badgesDomain.CreateCommand{
		Slug:         "bestseller",
		Color:        "#ff0000",
		Translations: []badgesDomain.Translation{{Locale: "en", Name: "Bestseller"}},
	})
	if err != nil {
		t.Fatalf("CreateBadge() error = %v, want the badge the facade just inserted", err)
	}
	if created == nil || created.Slug != "bestseller" || len(created.Translations) != 1 {
		t.Fatalf("CreateBadge() = %+v", created)
	}

	var badges, translations, auditEvents int64
	if err := db.Table("badges").Count(&badges).Error; err != nil || badges != 1 {
		t.Fatalf("badges rows = %d, err = %v", badges, err)
	}
	if err := db.Table("badge_translations").Count(&translations).Error; err != nil || translations != 1 {
		t.Fatalf("badge_translations rows = %d, err = %v", translations, err)
	}
	if err := db.Table("domain_events").Where("topic = ?", "admin.action.v1").Count(&auditEvents).Error; err != nil || auditEvents != 1 {
		t.Fatalf("audit events = %d, err = %v", auditEvents, err)
	}
}

// TestContentFacadeRollsBackTheBadgeWhenTheAuditEntryFails is the forensic
// bypass the facade exists to prevent, asserted directly: if the audit record
// cannot be written, the change it would have recorded must not survive.
func TestContentFacadeRollsBackTheBadgeWhenTheAuditEntryFails(t *testing.T) {
	db, actorID := newContentFacadeFixture(t)
	facade := newContentFacade(t, db, actorID, failingPublisher{})

	_, err := facade.CreateBadge(context.Background(), adminApplication.CatalogCommand{ActorUserID: actorID}, badgesDomain.CreateCommand{
		Slug:         "bestseller",
		Color:        "#ff0000",
		Translations: []badgesDomain.Translation{{Locale: "en", Name: "Bestseller"}},
	})
	if !errors.Is(err, errAuditUnavailable) {
		t.Fatalf("CreateBadge() error = %v, want the audit failure", err)
	}

	var badges int64
	if err := db.Table("badges").Count(&badges).Error; err != nil {
		t.Fatal(err)
	}
	if badges != 0 {
		t.Fatal("badge persisted although its audit entry failed: the mutation committed outside the facade transaction")
	}
}

// TestContentFacadeRollsBackSEOWhenTheAuditEntryFails covers the same property
// for SEO metadata, whose repository wrote through the pool rather than the
// caller's transaction and so committed independently of the audit record.
func TestContentFacadeRollsBackSEOWhenTheAuditEntryFails(t *testing.T) {
	db, actorID := newContentFacadeFixture(t)
	facade := newContentFacade(t, db, actorID, failingPublisher{})
	resourceID := uuid.New()

	_, err := facade.UpsertSEO(context.Background(), adminApplication.CatalogCommand{ActorUserID: actorID}, seoDomain.UpsertCommand{
		ResourceType: "product",
		ResourceID:   resourceID,
		Locale:       "en",
		Title:        "Hand cream",
	})
	if !errors.Is(err, errAuditUnavailable) {
		t.Fatalf("UpsertSEO() error = %v, want the audit failure", err)
	}

	var metadata int64
	if err := db.Table("seo_metadata").Count(&metadata).Error; err != nil {
		t.Fatal(err)
	}
	if metadata != 0 {
		t.Fatal("SEO metadata persisted although its audit entry failed: the mutation committed outside the facade transaction")
	}
}

// TestContentFacadeDeleteSEORollsBackWithItsAuditEntry guards the reverse
// direction: a moderator removing metadata must not be able to leave the
// deletion in place with no record of who made it.
func TestContentFacadeDeleteSEORollsBackWithItsAuditEntry(t *testing.T) {
	db, actorID := newContentFacadeFixture(t)
	resourceID := uuid.New()
	if err := db.Exec(`INSERT INTO seo_metadata (id, resource_type, resource_id, locale, title) VALUES (?, 'product', ?, 'en', 'Hand cream')`,
		uuid.New(), resourceID).Error; err != nil {
		t.Fatal(err)
	}
	facade := newContentFacade(t, db, actorID, failingPublisher{})

	if err := facade.DeleteSEO(context.Background(), adminApplication.CatalogCommand{ActorUserID: actorID}, "product", resourceID, "en"); !errors.Is(err, errAuditUnavailable) {
		t.Fatalf("DeleteSEO() error = %v, want the audit failure", err)
	}

	var metadata int64
	if err := db.Table("seo_metadata").Count(&metadata).Error; err != nil {
		t.Fatal(err)
	}
	if metadata != 1 {
		t.Fatal("SEO metadata deletion survived a failed audit entry: the delete committed outside the facade transaction")
	}
}

var errAuditUnavailable = errors.New("audit store is unavailable")

// failingPublisher stands in for the audit write failing after the mutation
// has already been made inside the transaction. Everything the facade did
// before it must disappear.
type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, eventsDomain.DomainEvent) error {
	return errAuditUnavailable
}

func newContentFacade(t *testing.T, db *gorm.DB, actorID uuid.UUID, publisher eventsDomain.TransactionalEventPublisher) *adminApplication.ContentAdminFacade {
	t.Helper()
	authorizer, err := adminApplication.NewAuthorizer(adminPostgres.NewRepository(db), cache.NewNoOpService(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	facade, err := adminApplication.NewContentAdminFacade(authorizer, adminPostgres.NewTransactionManager(db), publisher)
	if err != nil {
		t.Fatal(err)
	}
	return facade.WithBadges(badgesPostgres.NewRepository(db)).WithSEO(seoPostgres.NewRepository(db))
}

// newContentFacadeFixture returns a database with the RBAC, outbox, badges and
// SEO schemas applied, and an admin user holding badges:write and seo:write.
func newContentFacadeFixture(t *testing.T) (*gorm.DB, uuid.UUID) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	container, err := postgresContainer.Run(ctx, "postgres:16-alpine",
		postgresContainer.WithDatabase("content_facade_test"),
		postgresContainer.WithUsername("admin"),
		postgresContainer.WithPassword("admin"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)))
	if err != nil {
		t.Fatalf("start PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := applyContentFacadeSchema(db); err != nil {
		t.Fatal(err)
	}

	actorID, roleID := uuid.New(), uuid.New()
	badgesPermission, seoPermission := uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id) VALUES (?)", []any{actorID}},
		{"INSERT INTO admin_users (user_id, authorization_version) VALUES (?, 1)", []any{actorID}},
		{"INSERT INTO roles (id, code, name) VALUES (?, 'content-manager', 'Content manager')", []any{roleID}},
		{"INSERT INTO permissions (id, code, resource, action) VALUES (?, 'badges:write', 'badges', 'write')", []any{badgesPermission}},
		{"INSERT INTO permissions (id, code, resource, action) VALUES (?, 'seo:write', 'seo', 'write')", []any{seoPermission}},
		{"INSERT INTO admin_user_roles (user_id, role_id) VALUES (?, ?)", []any{actorID, roleID}},
		{"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", []any{roleID, badgesPermission}},
		{"INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", []any{roleID, seoPermission}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, actorID
}

func applyContentFacadeSchema(db *gorm.DB) error {
	// products is a stub: product_badges references it, but nothing in these
	// tests touches a product beyond satisfying that foreign key.
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE TABLE users (id UUID PRIMARY KEY); CREATE TABLE products (id UUID PRIMARY KEY);").Error; err != nil {
		return err
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("locate content facade migrations")
	}
	root := filepath.Join(filepath.Dir(file), "../../../")
	for _, migration := range []string{
		"migrations/modules/admin/000001_init_rbac.up.sql",
		"migrations/modules/badges/000001_init_badges.up.sql",
		"migrations/modules/seo/000001_init_seo.up.sql",
		"migrations/core/000005_add_event_outbox_and_order_contacts.up.sql",
		"migrations/core/000009_harden_outbox_retention_and_trace_context.up.sql",
	} {
		raw, err := os.ReadFile(filepath.Join(root, migration))
		if err != nil {
			return err
		}
		statements := string(raw)
		// The outbox migration also owns order_contact_details, which needs an
		// orders table this focused fixture deliberately does not create.
		if index := bytes.Index(raw, []byte("-- Order contact")); index >= 0 {
			statements = string(raw[:index])
		}
		if err := db.Exec(statements).Error; err != nil {
			return err
		}
	}
	return nil
}

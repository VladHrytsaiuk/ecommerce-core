//go:build integration

package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
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

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"
)

// Bootstrap is the Composition Root: nearly four hundred lines and thirteen
// module conditionals, and until this file nothing executed it. Every mistake
// in those branches was found by whichever deployment first enabled that
// combination, in production.
//
// These tests do not assert what each module does — that belongs to the
// module's own tests. They assert that the graph assembles at all, for the
// combinations a real store runs, and that it comes apart again cleanly.

func TestMain(m *testing.M) {
	// Bootstrap hands logger.Log to a dozen constructors and several workers
	// dereference it. main calls this before Bootstrap; a test that did not
	// would be exercising a process shape that never ships.
	logger.Init()
	code := m.Run()
	stopSharedDatabase()
	os.Exit(code)
}

func TestBootstrapAssemblesAStoreWithNothingOptionalEnabled(t *testing.T) {
	// The floor. Every module conditional takes its false branch, and the
	// result still has to be a store: a catalog, a cart, a checkout and orders.
	application := bootstrapFor(t, minimalModuleSet())

	for name, wired := range map[string]bool{
		"catalog products": application.CatalogProductService != nil,
		"cart":             application.CartService != nil,
		"checkout":         application.CheckoutService != nil,
		"orders":           application.OrderService != nil,
		"order workflow":   application.OrderWorkflowService != nil,
		"inventory":        application.InventoryService != nil,
	} {
		if !wired {
			t.Fatalf("%s is missing; the minimal store cannot take an order", name)
		}
	}
	// A worker with no handlers is not harmless: it claims this consumer's
	// deliveries. Nothing writes notification jobs here, so nothing may drain
	// them either.
	for name, built := range map[string]bool{
		"notifications worker":   application.NotificationWorker != nil,
		"notifications outbox":   application.OutboxWorker != nil,
		"notification retention": application.NotificationRetention != nil,
		"support":                application.SupportService != nil,
		"returns":                application.ReturnService != nil,
		"consent":                application.ConsentService != nil,
		"admin authorizer":       application.AdminAuthorizer != nil,
	} {
		if built {
			t.Fatalf("%s was built for a store that did not enable it", name)
		}
	}
}

func TestBootstrapAssemblesEveryModuleThatNeedsOnlyPostgres(t *testing.T) {
	// The combination that exercises the most conditionals at once, and the
	// one where a dependency between two modules would show.
	application := bootstrapFor(t, fullModuleSet())

	for name, wired := range map[string]bool{
		"returns":                  application.ReturnService != nil,
		"returns outbox":           application.ReturnsOutboxWorker != nil,
		"consent":                  application.ConsentService != nil,
		"reports queries":          application.ReportsQueryService != nil,
		"reports rebuilder":        application.ReportsRebuilder != nil,
		"reports outbox":           application.ReportsOutboxWorker != nil,
		"availability":             application.AvailabilityService != nil,
		"availability outbox":      application.AvailabilityOutboxWorker != nil,
		"abandoned cart":           application.AbandonedCartWorker != nil,
		"abandoned cart outbox":    application.AbandonedCartOutboxWorker != nil,
		"checkout contact capture": application.CheckoutContactCapture != nil,
		"notifications worker":     application.NotificationWorker != nil,
		"notifications outbox":     application.OutboxWorker != nil,
		"notification retention":   application.NotificationRetention != nil,
		"reviews":                  application.ReviewsService != nil,
		"wishlist":                 application.WishlistService != nil,
		"comparison":               application.ComparisonService != nil,
		"seo":                      application.SEOService != nil,
		"badges":                   application.BadgesService != nil,
		"admin authorizer":         application.AdminAuthorizer != nil,
		"admin audit outbox":       application.AdminAuditOutboxWorker != nil,
		"admin catalog facade":     application.CatalogAdminFacade != nil,
		"admin orders facade":      application.OrdersAdminFacade != nil,
		"admin content facade":     application.ContentAdminFacade != nil,
		"admin promos facade":      application.PromosAdminFacade != nil,
		"customer profiles":        application.CustomerProfileService != nil,
		"user profiles":            application.IdentityProfileService != nil,
	} {
		if !wired {
			t.Fatalf("%s was enabled but not wired", name)
		}
	}
	// Nothing here configures Meilisearch, S3, Cloudflare or an export
	// endpoint, so the modules that need them must stay off even though every
	// other module is on.
	if application.SearchService != nil || application.MediaUploadService != nil || application.VideoUploadService != nil || application.SyncDispatcher != nil {
		t.Fatal("a module requiring an external service was built without one")
	}
}

func TestBootstrapInstallsTheDefaultNotificationTemplates(t *testing.T) {
	// notification_templates ships empty. Booting is what fills it, and a store
	// with an empty table renders nothing and dispatches nothing.
	database := testDatabase(t)
	if err := database.Exec(`DELETE FROM notification_templates`).Error; err != nil {
		t.Fatal(err)
	}

	bootstrapFor(t, fullModuleSet())

	var installed int64
	if err := database.Raw(`SELECT COUNT(*) FROM notification_templates WHERE locale = ? AND is_active`, testStoreLocale).Scan(&installed).Error; err != nil {
		t.Fatal(err)
	}
	if installed == 0 {
		t.Fatalf("no templates for %q after boot; every message would die undeliverable", testStoreLocale)
	}
}

func TestBootstrapRefusesAModuleWhoseDependencyIsMissing(t *testing.T) {
	// NewStoreConfig checks this too. Bootstrap is the last thing between a
	// bad module list and a half-built graph, and it must not take the caller's
	// word for it.
	storeConfig := storeConfigFor(t, fullModuleSet())
	storeConfig.EnabledModules = withoutModule(storeConfig.EnabledModules, string(ModuleConsent))

	application, err := Bootstrap(testConfig(fullModuleSet()), storeConfig, testDatabase(t), testTokenMaker(t))

	if err == nil {
		t.Fatal("Bootstrap built abandoned_cart with no consent module to mint unsubscribe links")
	}
	// The message matters as much as the refusal. Without the guard Bootstrap
	// still fails — several modules reach for a nil consent service and one of
	// them says so — but the operator is told about an unsubscribe linker
	// rather than about the module list they have to change.
	if !strings.Contains(err.Error(), "ENABLED_MODULES") || !strings.Contains(err.Error(), string(ModuleConsent)) {
		t.Fatalf("Bootstrap() error = %v, want the module list named as the thing to fix", err)
	}
	if application != nil {
		t.Fatal("Bootstrap returned an application alongside an error")
	}
}

func TestBootstrapRefusesSupportWithoutRedis(t *testing.T) {
	// Support's ticket intake is public and abuse-sensitive. Its limiter has to
	// be shared across replicas; a per-process fallback would multiply the
	// quota by the replica count during a rollout.
	configuration := testConfig(supportModuleSet())
	configuration.RedisEnabled = false

	application, err := Bootstrap(configuration, storeConfigFor(t, supportModuleSet()), testDatabase(t), testTokenMaker(t))

	if err == nil {
		t.Fatal("Bootstrap built support with a per-process anti-spam limiter")
	}
	if application != nil {
		t.Fatal("Bootstrap returned an application alongside an error")
	}
}

func TestAStartedApplicationStopsWithinItsDeadline(t *testing.T) {
	// Start launches a goroutine per enabled worker. StopContext has to cancel
	// and join all of them: one worker that ignores cancellation turns SIGTERM
	// into a kill, mid-transaction.
	application := bootstrapFor(t, fullModuleSet())
	application.Start(context.Background())

	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		stopped <- application.StopContext(ctx)
	}()

	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("StopContext() error = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("StopContext never returned; a worker is not honouring cancellation")
	}
	// A SIGTERM racing a failed readiness probe stops the same application
	// twice. This does not prove much beyond that: no panic, no second error,
	// no block on a graph that is already down. That is the whole contract.
	if err := application.StopContext(context.Background()); err != nil {
		t.Fatalf("second StopContext() error = %v", err)
	}
}

// minimalModuleSet is the smallest list StoreConfig accepts — the mandatory
// modules and nothing else. It is derived rather than written out, so a module
// that becomes mandatory later cannot leave this test asserting a store that
// no longer boots.
func minimalModuleSet() []string { return withRequiredModules() }

// fullModuleSet is every module that needs nothing beyond PostgreSQL. Search
// needs Meilisearch, media needs object storage, video needs Cloudflare, sync
// needs an export endpoint and support needs Redis, so those five are covered
// by their own tests instead of here.
func fullModuleSet() []string {
	return withRequiredModules(
		string(ModuleAbandonedCart), string(ModuleAdmin), string(ModuleAvailability),
		string(ModuleBadges), string(ModuleCheckout), string(ModuleComparison),
		string(ModuleConsent), string(ModuleCustomers),
		string(ModuleNotifications), string(ModuleOrders), string(ModulePromos),
		string(ModuleReports), string(ModuleReturns), string(ModuleReviews),
		string(ModuleSEO), string(ModuleUserProfiles), string(ModuleWishlist),
	)
}

// supportModuleSet is support plus exactly what it declares it requires, so the
// only thing missing from the deployment is Redis.
func supportModuleSet() []string {
	return withRequiredModules(string(ModuleSupport), string(ModuleNotifications), string(ModuleAdmin))
}

func containsModuleName(modules []string, wanted string) bool {
	for _, module := range modules {
		if module == wanted {
			return true
		}
	}
	return false
}

func withoutModule(modules []string, removed string) []string {
	kept := make([]string, 0, len(modules))
	for _, module := range modules {
		if module != removed {
			kept = append(kept, module)
		}
	}
	return kept
}

func bootstrapFor(t *testing.T, modules []string) *Application {
	t.Helper()
	application, err := Bootstrap(testConfig(modules), storeConfigFor(t, modules), testDatabase(t), testTokenMaker(t))
	if err != nil {
		t.Fatalf("Bootstrap(%v) error = %v", modules, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if stopErr := application.StopContext(ctx); stopErr != nil {
			t.Errorf("StopContext() error = %v", stopErr)
		}
	})
	return application
}

func storeConfigFor(t *testing.T, modules []string) StoreConfig {
	t.Helper()
	storeConfig, err := NewStoreConfig(testConfig(modules))
	if err != nil {
		t.Fatalf("NewStoreConfig(%v) error = %v", modules, err)
	}
	return storeConfig
}

const testStoreLocale = "uk"

// testConfig is a deployment that satisfies every module in fullModuleSet:
// credentials for one payment provider (returns refunds through it), a
// marketing origin (unsubscribe links are minted against it) and an encryption
// key. Anything a module here does not need is left at its zero value so an
// accidental dependency on it fails rather than passes.
//
// It takes the module list because several settings are only valid when their
// module is on — a profile policy without user_profiles is rejected, which is
// the config guard doing its job.
func testConfig(modules []string) *config.Config {
	configuration := validConfig()
	configuration.EnabledModules = modules
	configuration.Env = "test"
	configuration.JWTSecret = "bootstrap-integration-secret-long-enough"
	configuration.AccessTokenDuration = 15 * time.Minute
	configuration.CORSAllowOrigins = []string{"https://store.example"}
	configuration.FrontendURL = "https://store.example"
	configuration.APIRateLimitPerMin = 100
	configuration.WebhookRatePerMin = 100
	configuration.RequestTimeout = 30 * time.Second
	configuration.ShutdownTimeout = 40 * time.Second
	// Port zero: the management listener is real, and a fixed port would make
	// this test fail for whoever else is using it.
	configuration.ManagementAddr = "127.0.0.1:0"
	configuration.RedisEnabled = false
	configuration.NotificationEmailProvider = "mock"
	configuration.NotificationEncryptionKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32))
	configuration.OutboxDoneRetention = 30 * 24 * time.Hour
	configuration.OutboxArchiveRetention = 90 * 24 * time.Hour
	configuration.OutboxRetentionInterval = time.Hour
	configuration.NotificationSentRetention = 30 * 24 * time.Hour
	configuration.NotificationDeadRetention = 90 * 24 * time.Hour
	configuration.ReportsTimezone = "UTC"
	configuration.ComparisonMaxItems = 20
	if containsModuleName(modules, string(ModuleUserProfiles)) {
		configuration.ProfilePolicyJSON = `{"schema_version":1,"fields":[]}`
	}
	configuration.AbandonedCartDelays = []time.Duration{time.Hour, 24 * time.Hour}
	configuration.AbandonedCartMaxReminders = 2
	configuration.AbandonedCartRequireMarketingConsent = true
	configuration.PaymentProviders, configuration.PaymentDefault = []string{"liqpay"}, "liqpay"
	configuration.LiqPayPublicKey = "sandbox_public_key"
	configuration.LiqPayPrivateKey = "sandbox_private_key"
	configuration.LiqPayCallbackURL = "https://api.store.example/api/webhooks/payments/liqpay"
	return configuration
}

func testTokenMaker(t *testing.T) token.Maker {
	t.Helper()
	maker, err := token.NewJWTMaker("bootstrap-integration-secret-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	return maker
}

// One container for the whole package. Bootstrap is read-mostly against the
// schema, so the tests share a migrated database rather than paying four
// container starts to prove the same migrations twice.
var (
	sharedDatabaseOnce sync.Once
	sharedDatabase     *gorm.DB
	sharedDatabaseErr  error
	sharedContainer    testcontainers.Container
)

func testDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	sharedDatabaseOnce.Do(func() { sharedDatabase, sharedDatabaseErr = startSharedDatabase() })
	if sharedDatabaseErr != nil {
		t.Fatalf("start migrated PostgreSQL: %v", sharedDatabaseErr)
	}
	return sharedDatabase
}

func startSharedDatabase() (*gorm.DB, error) {
	ctx := context.Background()
	container, err := containerPostgres.Run(ctx, "postgres:16-alpine",
		containerPostgres.WithDatabase("bootstrap_test"),
		containerPostgres.WithUsername("bootstrap"),
		containerPostgres.WithPassword("bootstrap"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		return nil, err
	}
	sharedContainer = container

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, err
	}
	// Every module's schema, not only the ones fullModuleSet boots: the module
	// list under test is a runtime choice, and migrating all of them keeps
	// adding a case here from also meaning editing this function.
	if err := migrateEveryModule(databaseURL); err != nil {
		return nil, err
	}
	return gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
}

func stopSharedDatabase() {
	if sharedContainer != nil {
		_ = sharedContainer.Terminate(context.Background())
	}
}

func migrateEveryModule(databaseURL string) error {
	root := repositoryRootForBootstrap()
	modules, err := os.ReadDir(filepath.Join(root, "migrations", "modules"))
	if err != nil {
		return fmt.Errorf("read module migrations: %w", err)
	}
	names := make([]string, 0, len(modules))
	for _, entry := range modules {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	// Core owns the tables the modules' foreign keys point at, so it goes
	// first — the same order cmd/migrate applies on startup.
	steps := []struct{ dir, table string }{{filepath.Join(root, "migrations", "core"), "schema_migrations"}}
	for _, name := range names {
		steps = append(steps, struct{ dir, table string }{
			filepath.Join(root, "migrations", "modules", name), "schema_migrations_module_" + name,
		})
	}
	for _, step := range steps {
		if err := migrateBootstrapDir(step.dir, databaseURL, step.table); err != nil {
			return err
		}
	}
	return nil
}

func migrateBootstrapDir(dir, databaseURL, table string) error {
	runner, err := migrate.New("file://"+dir, bootstrapMigrationURL(databaseURL, table))
	if err != nil {
		return fmt.Errorf("create migration runner for %s: %w", dir, err)
	}
	defer func() { _, _ = runner.Close() }()
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations in %s: %w", dir, err)
	}
	return nil
}

func bootstrapMigrationURL(databaseURL, table string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		panic(err)
	}
	query := parsed.Query()
	query.Set("x-migrations-table", table)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func repositoryRootForBootstrap() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

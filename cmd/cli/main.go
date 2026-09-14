// Command cli contains maintenance commands that must not be exposed over HTTP.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/mail"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	catalogApp "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/application"
	catalogPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/catalog/repository/postgres"
	notificationsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/domain"
	notificationsPostgres "github.com/VladHrytsaiuk/ecommerce-core/internal/notifications/repository/postgres"
	platformConfig "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	platformDB "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/encryption"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/password"
	searchCatalog "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/catalog"
	searchMeili "github.com/VladHrytsaiuk/ecommerce-core/internal/search/adapter/meilisearch"
	searchApp "github.com/VladHrytsaiuk/ecommerce-core/internal/search/application"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("load .env: %v", err)
	}
	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/cli create-owner ... | grant-superadmin -email admin@example.com | search-reindex [-batch-size 200] | notifications-reencrypt [-batch-size 500]")
	}
	if os.Args[1] == "grant-superadmin" {
		grantSuperAdmin()
		return
	}
	if os.Args[1] == "search-reindex" {
		searchReindex()
		return
	}
	if os.Args[1] == "notifications-reencrypt" {
		reencryptNotifications()
		return
	}
	if os.Args[1] != "create-owner" {
		log.Fatal("unknown command")
	}

	flags := flag.NewFlagSet("create-owner", flag.ExitOnError)
	emailFlag := flags.String("email", os.Getenv("OWNER_EMAIL"), "owner email (or OWNER_EMAIL)")
	passwordFlag := flags.String("password", os.Getenv("OWNER_PASSWORD"), "owner password (or OWNER_PASSWORD)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}

	databaseURL := strings.TrimSpace(os.Getenv("DB_URL"))
	email := strings.ToLower(strings.TrimSpace(*emailFlag))
	plainPassword := *passwordFlag
	if databaseURL == "" {
		log.Fatal("DB_URL is required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		log.Fatalf("invalid owner email: %v", err)
	}
	if len(plainPassword) < 12 || len(plainPassword) > 72 {
		log.Fatal("owner password must contain 12 to 72 bytes")
	}

	hash, err := password.HashPassword(plainPassword)
	if err != nil {
		log.Fatalf("hash owner password: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: databaseURL, PreferSimpleProtocol: true}), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect PostgreSQL: %v", err)
	}

	owner := ownerRecord{ID: uuid.New(), Email: email, PasswordHash: hash, Role: "owner", Status: "active"}
	result := db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "email"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("email IS NOT NULL")}},
		DoNothing:   true,
	}).Create(&owner)
	if result.Error != nil {
		log.Fatalf("create owner: %v", result.Error)
	}
	if result.RowsAffected == 0 {
		log.Fatalf("owner %q already exists; refusing to overwrite credentials", email)
	}
	fmt.Printf("owner created: %s (%s)\n", owner.Email, owner.ID)
}

// searchReindex rebuilds the disposable Meilisearch read projection in bounded
// batches. PostgreSQL remains the source of truth; this command never mutates
// catalog data and is deliberately not exposed through the public API.
func searchReindex() {
	flags := flag.NewFlagSet("search-reindex", flag.ExitOnError)
	batchSize := flags.Int("batch-size", 200, "documents per Meilisearch batch (1-1000)")
	if err := flags.Parse(os.Args[2:]); err != nil {
		log.Fatal(err)
	}
	if *batchSize < 1 || *batchSize > 1000 {
		log.Fatal("batch-size must be between 1 and 1000")
	}
	cfg := platformConfig.Load()
	storeConfig, err := app.NewStoreConfig(cfg)
	if err != nil {
		log.Fatalf("load store configuration: %v", err)
	}
	if !storeConfig.Modules().Has(app.ModuleSearch) {
		log.Fatal("search-reindex requires search in ENABLED_MODULES")
	}
	database, err := platformDB.Connect(cfg.DBURL, platformDB.DefaultPoolConfig())
	if err != nil {
		log.Fatalf("connect PostgreSQL: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		log.Fatalf("obtain PostgreSQL pool: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	index, err := searchMeili.New(ctx, searchMeili.Config{
		URL: cfg.SearchURL, MasterKey: cfg.SearchMasterKey,
		IndexUID: cfg.SearchIndexPrefix + "_" + storeConfig.Code + "_products",
	})
	if err != nil {
		log.Fatalf("configure search index: %v", err)
	}
	defer func() { _ = index.Close() }()
	products := catalogApp.NewProductService(catalogPostgres.NewProductRepository(database), storeConfig.SupportedLocales)
	reindexer, err := searchApp.NewReindexer(searchCatalog.NewSnapshotProvider(products), index)
	if err != nil {
		log.Fatalf("configure search reindex: %v", err)
	}
	count, err := reindexer.Run(ctx, *batchSize)
	if err != nil {
		log.Fatalf("reindex search documents: %v", err)
	}
	fmt.Printf("search reindex complete: %d products\n", count)
}

// grantSuperAdmin is deliberately a local maintenance command: no HTTP route
// can bootstrap privileged access. The Admin migration must already be applied.
func grantSuperAdmin() {
	flags := flag.NewFlagSet("grant-superadmin", flag.ExitOnError)
	emailFlag := flags.String("email", "", "existing user email")
	bootstrapToken := flags.String("bootstrap-token", "", "required after initial SuperAdmin bootstrap (or ADMIN_BOOTSTRAP_TOKEN)")
	_ = flags.Parse(os.Args[2:])
	email := strings.ToLower(strings.TrimSpace(*emailFlag))
	if _, err := mail.ParseAddress(email); err != nil {
		log.Fatal("valid -email is required")
	}
	databaseURL := strings.TrimSpace(os.Getenv("DB_URL"))
	if databaseURL == "" {
		log.Fatal("DB_URL is required")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: databaseURL, PreferSimpleProtocol: true}), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect PostgreSQL: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		// Serialize the bootstrap decision across CLI processes and hosts. The
		// lock is automatically released with this transaction.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext('admin:superadmin-bootstrap'))`).Error; err != nil {
			return err
		}
		var existing int64
		if err := tx.Raw(`SELECT COUNT(*) FROM admin_user_roles aur JOIN roles r ON r.id = aur.role_id JOIN admin_users au ON au.user_id = aur.user_id WHERE r.code = 'super_admin' AND au.is_active = TRUE`).Scan(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			expected := strings.TrimSpace(os.Getenv("ADMIN_BOOTSTRAP_TOKEN"))
			if expected == "" || *bootstrapToken != expected {
				return fmt.Errorf("SuperAdmin already exists; ADMIN_BOOTSTRAP_TOKEN is required")
			}
		}
		var userID uuid.UUID
		if err := tx.Raw("SELECT id FROM users WHERE email = ?", email).Scan(&userID).Error; err != nil || userID == uuid.Nil {
			return fmt.Errorf("user not found")
		}
		if err := tx.Exec("INSERT INTO admin_users (user_id, is_active, authorization_version) VALUES (?, TRUE, 1) ON CONFLICT (user_id) DO UPDATE SET is_active = TRUE, authorization_version = admin_users.authorization_version + 1", userID).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO admin_user_roles (user_id, role_id) SELECT ?, id FROM roles WHERE code = 'super_admin' ON CONFLICT DO NOTHING", userID).Error
	}); err != nil {
		log.Fatalf("grant SuperAdmin: %v", err)
	}
	fmt.Printf("SuperAdmin granted: %s\n", email)
}

type ownerRecord struct {
	ID           uuid.UUID `gorm:"column:id"`
	Email        string    `gorm:"column:email"`
	PasswordHash string    `gorm:"column:password_hash"`
	Role         string    `gorm:"column:role"`
	Status       string    `gorm:"column:status"`
}

func (ownerRecord) TableName() string { return "users" }

// reencryptNotifications is stage 3 of docs/design/notification-payload-encryption.md:
// it moves scheduled jobs written before the change out of their plaintext
// columns and rewrites the dedupe keys those columns were concatenated into.
//
// A local command rather than a migration, because the encryption key lives in
// the application: SQL cannot produce ciphertext, and a migration that tried
// would have to be handed the key. Safe to run repeatedly and while the store
// is serving — batches are bounded and skip rows the dispatcher holds.
func reencryptNotifications() {
	flags := flag.NewFlagSet("notifications-reencrypt", flag.ExitOnError)
	batchSize := flags.Int("batch-size", 500, "rows per batch")
	if err := flags.Parse(os.Args[2:]); err != nil {
		log.Fatalf("parse flags: %v", err)
	}
	cfg := platformConfig.Load()
	storeConfig, err := app.NewStoreConfig(cfg)
	if err != nil {
		log.Fatalf("load store configuration: %v", err)
	}
	if !storeConfig.Modules().Has(app.ModuleNotifications) {
		log.Fatal("notifications-reencrypt requires notifications in ENABLED_MODULES")
	}
	database, err := platformDB.Connect(cfg.DBURL, platformDB.DefaultPoolConfig())
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	pool, err := database.DB()
	if err != nil {
		log.Fatalf("obtain PostgreSQL pool: %v", err)
	}
	defer func() { _ = pool.Close() }()

	cipher, err := encryption.NewAESGCM(cfg.NotificationEncryptionKey)
	if err != nil {
		log.Fatalf("configure notifications encryption: %v", err)
	}
	dedupe, err := notificationsDomain.NewDedupeKeyer(cfg.NotificationEncryptionKey)
	if err != nil {
		log.Fatalf("configure notifications dedupe key: %v", err)
	}
	repository := notificationsPostgres.NewRepository(database).
		WithDefaultLocale(storeConfig.DefaultLocale).
		WithEncryption(cipher, dedupe)

	ctx := context.Background()
	total := 0
	for {
		converted, err := repository.ReencryptPlaintext(ctx, *batchSize)
		if err != nil {
			log.Fatalf("re-encrypt notification jobs: %v", err)
		}
		if converted == 0 {
			break
		}
		total += converted
		fmt.Printf("re-encrypted %d notification jobs (%d so far)\n", converted, total)
	}
	remaining, err := repository.CountPlaintext(ctx)
	if err != nil {
		log.Fatalf("count remaining plaintext jobs: %v", err)
	}
	fmt.Printf("re-encryption complete: %d converted, %d still in plaintext\n", total, remaining)
	if remaining > 0 {
		// Rows the dispatcher held during every pass. Run again; the final
		// migration will refuse while any remain.
		log.Fatal("some jobs were locked throughout; run this again before applying the migration that drops the plaintext columns")
	}
}

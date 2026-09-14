package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/app"
	apphttp "github.com/VladHrytsaiuk/ecommerce-core/internal/http"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/security/token"

	// Blank import: the generated Swagger package registers itself on init.
	swaggerDocs "github.com/VladHrytsaiuk/ecommerce-core/docs/api"
)

// @title           AquaWheel Store API
// @version         1.0
// @description     Backend API for AquaWheel e-commerce project.
// @tag.name Admin
// @tag.description Адмін-панель (модерація, налаштування)
// @tag.name Auth
// @tag.description Аутентифікація та реєстрація
// @tag.name Users
// @tag.description Профіль користувача
// @tag.name Addresses
// @tag.description Збережені адреси доставки
// @tag.name Products
// @tag.description Каталог товарів (товари, бренди, категорії, відгуки)
// @BasePath        /
//
// @securityDefinitions.apikey bearerAuth
// @in header
// @name Authorization
// @description Введіть токен у форматі: Bearer {your_token}
// workerShutdownTimeout covers a worker acknowledging the unit of work it had
// already claimed. Unlike the HTTP drain this does not scale with request
// budgets: the workers bound their own finalization at three seconds.
const workerShutdownTimeout = 5 * time.Second

func main() {
	// 1. The logger first: everything below reports through it.
	logger.Init()
	// Flush whatever is still buffered before the process exits.
	defer func() {
		_ = logger.Log.Sync()
	}()

	// The context every worker and the HTTP server shut down with.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	logger.Log.Info("🚀 Starting AquaWheel Store Backend...")

	// 2. Configuration, which fails the boot rather than defaulting.
	cfg := config.Load()
	storeConfig, err := app.NewStoreConfig(cfg)
	if err != nil {
		logger.Log.Fatalw("❌ Invalid store configuration", "error", err)
	}

	// 3. PostgreSQL.
	database, err := db.Connect(cfg.DBURL, db.PoolConfig(cfg.DBPool))
	if err != nil {
		logger.Log.Fatal("Cannot connect to PostgreSQL")
	}
	sqlDB, err := database.DB()
	if err != nil {
		logger.Log.Fatal("Cannot obtain PostgreSQL pool")
	}
	defer func() { _ = sqlDB.Close() }()
	logger.Log.Info("✅ Database connection established")

	// 4. The token maker.
	//
	// Tokens are scoped to STORE_CODE and not to JWT_SECRET alone: two copies of
	// this core that end up sharing a secret would otherwise accept each other's
	// tokens. Changing STORE_CODE invalidates every existing session of this
	// deployment.
	issuer, audience := token.IdentityForStore(storeConfig.Code)
	tokenMaker, err := token.NewJWTMakerFor(cfg.JWTSecret, issuer, audience)
	if err != nil {
		logger.Log.Fatalw("❌ Cannot create token maker", "error", err)
	}

	// 5. The Composition Root assembles everything; the HTTP package only
	// registers routes over what it is handed.
	application, err := app.Bootstrap(cfg, storeConfig, database, tokenMaker)
	if err != nil {
		logger.Log.Fatalw("❌ Invalid application configuration", "error", err)
	}
	// Worker lifecycle is stopped after HTTP requests drain, not immediately on
	// SIGTERM. This prevents a shutdown signal from cancelling work needed by an
	// in-flight checkout request.
	application.Start(context.Background())
	r := apphttp.InitRouter(application)

	// Swagger UI. An empty host makes it use relative paths and resolve the
	// host from the page it is served on.
	if cfg.APIHost != "" {
		swaggerDocs.SwaggerInfo.Host = cfg.APIHost
	} else {
		swaggerDocs.SwaggerInfo.Host = ""
	}

	// Schemes. Left empty, Swagger UI follows the scheme of the current page;
	// a configured host lists https first.
	if cfg.APIHost != "" && cfg.APIHost != "localhost:8080" {
		swaggerDocs.SwaggerInfo.Schemes = []string{"https", "http"}
	} else {
		// Locally, or with no host configured, autodetection is the right answer.
		swaggerDocs.SwaggerInfo.Schemes = []string{}
	}

	// 6. Serve.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Log.Infof("📡 API server is listening on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatalw("❌ Failed to start server", "error", err)
		}
	}()

	// Block until a termination signal arrives.
	<-ctx.Done()

	logger.Log.Info("🛑 Shutting down server...")
	// Long enough for a request using its whole budget to finish. This was five
	// seconds against a thirty-second request timeout, so SIGTERM cut off work
	// the server itself had said it would allow; the value is derived from that
	// timeout now rather than typed independently of it.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Continue with worker cancellation even when a handler exceeded the HTTP
		// deadline. Fatalw would call os.Exit and skip that controlled cleanup.
		logger.Log.Errorw("❌ Server forced to shutdown", "error", err)
	}
	// Workers are cancellable and their transactions roll back, so this only has
	// to cover a claimed unit of work being acknowledged: both outbox workers
	// bound that at three seconds on a detached context.
	workersShutdownCtx, stopWorkers := context.WithTimeout(context.Background(), workerShutdownTimeout)
	defer stopWorkers()
	if err := application.StopContext(workersShutdownCtx); err != nil {
		logger.Log.Errorw("⚠️ Background workers did not stop before deadline", "error", err)
	}

	logger.Log.Info("👋 Server exited properly")
}

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

	// Обов'язково імпортуємо папку api, яку згенерує Swagger
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
func main() {
	// 1. Ініціалізуємо логер ПЕРШИМ
	logger.Init()
	// Sync вимиває залишки логів з буфера перед завершенням програми
	defer func() {
		_ = logger.Log.Sync()
	}()

	// Створюємо контекст для Graceful Shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	logger.Log.Info("🚀 Starting AquaWheel Store Backend...")

	// 2. Завантажуємо конфігурацію
	cfg := config.Load()
	storeConfig, err := app.NewStoreConfig(cfg)
	if err != nil {
		logger.Log.Fatalw("❌ Invalid store configuration", "error", err)
	}

	// 3. Підключення до бази даних
	database, err := db.Connect(cfg.DBURL, db.DefaultPoolConfig())
	if err != nil {
		logger.Log.Fatal("Cannot connect to PostgreSQL")
	}
	sqlDB, err := database.DB()
	if err != nil {
		logger.Log.Fatal("Cannot obtain PostgreSQL pool")
	}
	defer func() { _ = sqlDB.Close() }()
	logger.Log.Info("✅ Database connection established")

	// 4. Ініціалізація Token Maker для JWT
	tokenMaker, err := token.NewJWTMaker(cfg.JWTSecret)
	if err != nil {
		logger.Log.Fatalw("❌ Cannot create token maker", "error", err)
	}

	// 5. Composition Root збирає залежності, HTTP пакет лише реєструє маршрути.
	application, err := app.Bootstrap(cfg, storeConfig, database, tokenMaker)
	if err != nil {
		logger.Log.Fatalw("❌ Invalid application configuration", "error", err)
	}
	// Worker lifecycle is stopped after HTTP requests drain, not immediately on
	// SIGTERM. This prevents a shutdown signal from cancelling work needed by an
	// in-flight checkout request.
	application.Start(context.Background())
	r := apphttp.InitRouter(application)

	// Налаштування Swagger
	// Якщо APIHost порожній, очищуємо його, щоб Swagger UI використовував відносні шляхи (автовизначення хоста)
	if cfg.APIHost != "" {
		swaggerDocs.SwaggerInfo.Host = cfg.APIHost
	} else {
		swaggerDocs.SwaggerInfo.Host = ""
	}

	// Налаштовуємо протоколи (http/https)
	// Якщо Schemes порожній, Swagger UI автоматично використовує протокол поточної сторінки.
	// Для сервера ставимо https першим.
	if cfg.APIHost != "" && cfg.APIHost != "localhost:8080" {
		swaggerDocs.SwaggerInfo.Schemes = []string{"https", "http"}
	} else {
		// На локалхості або якщо хост не вказано, краще залишити порожнім для автовизначення
		swaggerDocs.SwaggerInfo.Schemes = []string{}
	}

	// 6. Запуск сервера
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

	// Очікуємо сигнал завершення
	<-ctx.Done()

	logger.Log.Info("🛑 Shutting down server...")
	// Даємо серверу 5 секунд на завершення поточних запитів
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Continue with worker cancellation even when a handler exceeded the HTTP
		// deadline. Fatalw would call os.Exit and skip that controlled cleanup.
		logger.Log.Errorw("❌ Server forced to shutdown", "error", err)
	}
	workersShutdownCtx, stopWorkers := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopWorkers()
	if err := application.StopContext(workersShutdownCtx); err != nil {
		logger.Log.Errorw("⚠️ Background workers did not stop before deadline", "error", err)
	}

	logger.Log.Info("👋 Server exited properly")
}

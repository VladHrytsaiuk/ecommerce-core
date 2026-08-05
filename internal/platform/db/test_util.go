//go:build integration && legacy

package db

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

// SetupTestDB створює ізольовану базу даних у Docker для тестів та накочує всі міграції.
// t: об'єкт тесту для логування та фатальних помилок.
// Повертає з'єднання GORM та функцію очищення (teardown).
func SetupTestDB(t *testing.T) (*gorm.DB, func()) {
	ctx := context.Background()

	// 1. Запуск PostgreSQL контейнера
	dbName := "testdb"
	dbUser := "user"
	dbPassword := "password"

	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	// Функція очищення
	cleanup := func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Errorf("failed to terminate container: %s", err)
		}
	}

	// 2. Отримання DSN від контейнера
	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get connection string: %s", err)
	}

	// 3. Накочування міграцій
	// Шукаємо шлях до папки migrations відносно поточного файлу
	_, filename, _, _ := runtime.Caller(0)
	// Шлях від internal/platform/db/test_util.go до кореня/migrations
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations")

	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		connStr,
	)
	if err != nil {
		cleanup()
		t.Fatalf("failed to create migration instance: %s", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		cleanup()
		t.Fatalf("failed to run migrations: %s", err)
	}

	// 4. Підключення через GORM
	db := Connect(connStr)

	// 5. Seeding базових даних
	seedBasicData(t, db)

	return db, cleanup
}

func seedBasicData(t *testing.T, db *gorm.DB) {
	// Додаємо базову роль, щоб уникнути помилок Foreign Key
	// Використовуємо Raw SQL, щоб не залежати від доменних моделей тут
	err := db.Exec(`INSERT INTO role (id, name, description) VALUES (1, 'Customer', 'Default customer role') ON CONFLICT DO NOTHING`).Error
	if err != nil {
		t.Fatalf("failed to seed roles: %v", err)
	}

	// Сетапимо базові мови для тестів, оскільки вони вимагаються для Foreign Key в таблицях перекладів
	err = db.Exec(`INSERT INTO language (code, name) VALUES ('uk', 'Ukrainian'), ('en', 'English') ON CONFLICT DO NOTHING`).Error
	if err != nil {
		t.Fatalf("failed to seed languages: %v", err)
	}
}

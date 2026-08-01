//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestConnectIntegration(t *testing.T) {
	// 1. Створюємо контекст для керування життєвим циклом контейнера
	ctx := context.Background()

	// 2. Описуємо та запускаємо контейнер з PostgreSQL
	dbName := "testdb"
	dbUser := "user"
	dbPassword := "password"

	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine", // Використовуємо легкий образ
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

	// 3. Гарантуємо, що контейнер зупиниться після завершення тесту
	defer func() {
		if err := postgresContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}()

	// 4. Отримуємо Connection String (DSN) від запущеного контейнера
	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	// 5. ТЕСТУЄМО нашу функцію Connect з internal/platform/db/postgres.go
	db := Connect(connStr)

	// 6. ПЕРЕВІРКИ
	assert.NotNil(t, db, "Database connection should not be nil")

	sqlDB, err := db.DB()
	assert.NoError(t, err)

	// Перевіряємо, чи база реально відповідає на Ping
	err = sqlDB.Ping()
	assert.NoError(t, err, "Database should be reachable")
}

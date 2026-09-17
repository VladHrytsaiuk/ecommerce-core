//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	pool := testPoolConfig()
	db, err := Connect(connStr, pool)

	// 6. ПЕРЕВІРКИ
	require.NoError(t, err)
	require.NotNil(t, db, "Database connection should not be nil")

	sqlDB, err := db.DB()
	require.NoError(t, err)

	// Перевіряємо, чи база реально відповідає на Ping
	assert.NoError(t, sqlDB.Ping(), "Database should be reachable")

	// Пул налаштовується самим Connect, тож конфігурація має дійти до драйвера.
	assert.Equal(t, pool.MaxOpenConns, sqlDB.Stats().MaxOpenConnections)
}

func TestConnectRejectsInvalidPoolConfigIntegration(t *testing.T) {
	ctx := context.Background()
	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("user"),
		postgres.WithPassword("password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	defer func() { _ = postgresContainer.Terminate(ctx) }()

	connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	// Більше idle- ніж open-з'єднань драйвер мовчки обріже, тож така
	// конфігурація має бути відхилена на старті, а не проявитись під навантаженням.
	invalid := testPoolConfig()
	invalid.MaxIdleConns = invalid.MaxOpenConns + 1

	db, err := Connect(connStr, invalid)
	assert.Error(t, err)
	assert.Nil(t, db)
}

// testPoolConfig is a valid pool for exercising Connect. Production defaults
// are config's; these tests only need a pool Connect accepts.
func testPoolConfig() PoolConfig {
	return PoolConfig{MaxOpenConns: 25, MaxIdleConns: 10, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute}
}

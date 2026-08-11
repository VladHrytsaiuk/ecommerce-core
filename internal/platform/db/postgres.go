package db

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func DefaultPoolConfig() PoolConfig {
	return PoolConfig{MaxOpenConns: 25, MaxIdleConns: 10, ConnMaxLifetime: 30 * time.Minute, ConnMaxIdleTime: 5 * time.Minute}
}

// Connect creates the production database pool. SQL text and bound values are
// deliberately disabled because they may contain PII or credentials.
func Connect(dsn string, pool PoolConfig) (*gorm.DB, error) {
	// PreferSimpleProtocol вимикає неявне кешування prepared statements у драйвері pgx,
	// що вирішує помилку "prepared statement already exists (SQLSTATE 42P05)".
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain PostgreSQL pool")
	}
	if pool.MaxOpenConns <= 0 || pool.MaxIdleConns < 0 || pool.MaxIdleConns > pool.MaxOpenConns || pool.ConnMaxLifetime <= 0 || pool.ConnMaxIdleTime <= 0 {
		return nil, fmt.Errorf("invalid PostgreSQL pool configuration")
	}
	sqlDB.SetMaxOpenConns(pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(pool.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(pool.ConnMaxIdleTime)
	return db, nil
}

// TxKey — ключ для зберігання транзакції GORM у контексті.
// Використовується для прозорої передачі транзакції між різними репозиторіями.
type TxKey struct{}

// GetTx повертає транзакцію з контексту, якщо вона там є. Інакше повертає defaultDB.
func GetTx(ctx context.Context, defaultDB *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(TxKey{}).(*gorm.DB); ok {
		return tx
	}
	return defaultDB
}

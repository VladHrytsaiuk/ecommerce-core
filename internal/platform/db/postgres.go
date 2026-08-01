package db

import (
	"context"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(dsn string) *gorm.DB {
	// Підключаємось до БД з увімкненим логуванням SQL-запитів
	// PreferSimpleProtocol вимикає неявне кешування prepared statements у драйвері pgx,
	// що вирішує помилку "prepared statement already exists (SQLSTATE 42P05)".
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Successfully connected to PostgreSQL (Supabase)!")

	return db
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

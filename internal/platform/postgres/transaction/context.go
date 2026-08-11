// Package transaction carries a database transaction across module adapters.
// It is intentionally infrastructure-only and must only be used in a caller
// that already owns the transaction lifetime.
package transaction

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var ErrMissing = errors.New("postgres transaction is missing from context")

type key struct{}

func WithContext(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, key{}, db)
}
func FromContext(ctx context.Context) (*gorm.DB, error) {
	db, ok := ctx.Value(key{}).(*gorm.DB)
	if !ok || db == nil {
		return nil, ErrMissing
	}
	return db, nil
}

// Within joins an existing unit of work instead of creating a nested GORM
// savepoint. Repositories use it for operations that may be called by a
// cross-module facade while retaining standalone transactional behaviour.
func Within(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	if tx, err := FromContext(ctx); err == nil {
		return fn(tx.WithContext(ctx))
	}
	return db.WithContext(ctx).Transaction(fn)
}

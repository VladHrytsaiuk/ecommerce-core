package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

// TransactionManager adapts GORM transaction ownership to the Admin facade.
type TransactionManager struct{ db *gorm.DB }

func NewTransactionManager(db *gorm.DB) *TransactionManager { return &TransactionManager{db: db} }

func (m *TransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if m == nil || m.db == nil || fn == nil {
		return fmt.Errorf("admin transaction is not configured")
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(transaction.WithContext(ctx, tx))
	})
}

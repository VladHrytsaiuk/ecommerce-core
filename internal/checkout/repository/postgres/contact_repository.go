package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	checkout "github.com/VladHrytsaiuk/ecommerce-core/internal/checkout/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
)

type ContactRepository struct{ db *gorm.DB }

func NewContactRepository(db *gorm.DB) *ContactRepository { return &ContactRepository{db: db} }

type contactRecord struct {
	CartID    uuid.UUID `gorm:"primaryKey"`
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (contactRecord) TableName() string { return "checkout_contacts" }

func (r *ContactRepository) UpsertContact(ctx context.Context, contact checkout.ContactSnapshot) error {
	return r.database(ctx).Exec(`INSERT INTO checkout_contacts (cart_id, email, created_at, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (cart_id) DO UPDATE SET email = EXCLUDED.email, updated_at = CURRENT_TIMESTAMP`, contact.CartID, contact.Email).Error
}

func (r *ContactRepository) FindContact(ctx context.Context, cartID uuid.UUID) (*checkout.ContactSnapshot, error) {
	var record contactRecord
	if err := r.database(ctx).First(&record, "cart_id = ?", cartID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &checkout.ContactSnapshot{CartID: record.CartID, Email: record.Email}, nil
}

func (r *ContactRepository) database(ctx context.Context) *gorm.DB {
	if tx, err := transaction.FromContext(ctx); err == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

var _ checkout.ContactRepository = (*ContactRepository)(nil)

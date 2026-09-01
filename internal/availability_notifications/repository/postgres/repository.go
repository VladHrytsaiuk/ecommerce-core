package postgres

import (
	"context"
	availability "github.com/VladHrytsaiuk/ecommerce-core/internal/availability_notifications/domain"
	transaction "github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type record struct {
	ID         uuid.UUID
	VariantID  uuid.UUID
	Email      string
	Status     string
	CreatedAt  time.Time
	NotifiedAt *time.Time
}

func (record) TableName() string { return "stock_subscriptions" }
func (r *Repository) database(ctx context.Context) *gorm.DB {
	if tx, e := transaction.FromContext(ctx); e == nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}
func (r *Repository) Create(ctx context.Context, s availability.Subscription) (*availability.Subscription, bool, error) {
	s.Email = strings.ToLower(strings.TrimSpace(s.Email))
	rec := record{ID: uuid.New(), VariantID: s.VariantID, Email: s.Email, Status: string(availability.StatusPending), CreatedAt: time.Now().UTC()}
	result := r.database(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "variant_id"}, {Name: "email"}}, DoNothing: true}).Create(&rec)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		var old record
		if e := r.database(ctx).Where("variant_id = ? AND email = ? AND status = 'pending'", s.VariantID, s.Email).First(&old).Error; e != nil {
			return nil, false, e
		}
		return toDomain(old), false, nil
	}
	return toDomain(rec), true, nil
}
func (r *Repository) MarkPendingNotified(ctx context.Context, variantID uuid.UUID, at time.Time) ([]availability.Subscription, error) {
	var rows []record
	err := r.database(ctx).Raw(`UPDATE stock_subscriptions SET status='notified', notified_at=? WHERE variant_id=? AND status='pending' RETURNING id, variant_id, email, status, created_at, notified_at`, at, variantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]availability.Subscription, 0, len(rows))
	for _, v := range rows {
		out = append(out, *toDomain(v))
	}
	return out, nil
}
func toDomain(r record) *availability.Subscription {
	return &availability.Subscription{ID: r.ID, VariantID: r.VariantID, Email: r.Email, Status: availability.Status(r.Status), CreatedAt: r.CreatedAt}
}

var _ availability.Repository = (*Repository)(nil)

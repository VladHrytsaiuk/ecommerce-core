package postgres

import (
	"context"
	cart "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/postgres/transaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }

type row cart.Campaign

func (row) TableName() string { return "abandoned_cart_campaigns" }
func (r *Repository) ClaimDue(c context.Context, now time.Time) (*cart.Campaign, error) {
	var x row
	var token uuid.UUID
	e := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("(status='pending' AND due_at<=?) OR (status='processing' AND locked_at<=?)", now, now.Add(-5*time.Minute)).Order("due_at,id").Limit(1).Find(&x).Error; e != nil {
			return e
		}
		if x.ID == uuid.Nil {
			return nil
		}
		// A fresh token per claim: the takeover branch above can hand this
		// campaign to another worker while the previous one is still running,
		// and that worker must not be able to finish against the new claim.
		token = uuid.New()
		return tx.Model(&x).Updates(map[string]any{"status": "processing", "locked_at": now, "lock_token": token, "updated_at": now}).Error
	})
	if e != nil {
		return nil, e
	}
	if x.ID == uuid.Nil {
		return nil, nil
	}
	x.Status = "processing"
	x.LockedAt = now
	x.LockToken = token
	return (*cart.Campaign)(&x), nil
}

// Update writes the outcome of one pass. It is conditional on the claim's
// token, so a worker whose lease expired mid-pass changes nothing.
func (r *Repository) Update(c context.Context, x *cart.Campaign) error {
	return r.finalize(c, x.ID, x.LockToken, map[string]any{"status": x.Status, "due_at": x.DueAt, "locked_at": nil, "lock_token": nil, "updated_at": time.Now().UTC()})
}
func (r *Repository) Create(c context.Context, x cart.Campaign) error {
	return r.database(c).Create((*row)(&x)).Error
}
func (r *Repository) CreateOrReset(c context.Context, x cart.Campaign) error {
	return r.database(c).Exec(`INSERT INTO abandoned_cart_campaigns
 (id, cart_id, customer_id, contact_email, step, status, due_at, created_at, updated_at)
 VALUES (?, ?, ?, ?, ?, 'pending', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
 ON CONFLICT (cart_id, step) DO UPDATE
 SET customer_id = EXCLUDED.customer_id,
     contact_email = EXCLUDED.contact_email,
     status = 'pending', due_at = EXCLUDED.due_at,
     locked_at = NULL, updated_at = CURRENT_TIMESTAMP`, x.ID, x.CartID, x.CustomerID, x.ContactEmail, x.Step, x.DueAt).Error
}

// Requeue releases a claim whose pass failed. It takes the token for the same
// reason Update does: releasing a lease another worker now holds would let two
// passes run on the same campaign.
func (r *Repository) Requeue(c context.Context, id uuid.UUID, token uuid.UUID) error {
	return r.finalize(c, id, token, map[string]any{"status": "pending", "locked_at": nil, "lock_token": nil, "updated_at": time.Now().UTC()})
}

func (r *Repository) finalize(c context.Context, id, token uuid.UUID, values map[string]any) error {
	query := r.database(c).Model((*row)(nil)).Where("id=? AND status='processing'", id)
	if token != uuid.Nil {
		query = query.Where("lock_token=?", token)
	}
	return query.Updates(values).Error
}
func (r *Repository) database(c context.Context) *gorm.DB {
	if tx, e := transaction.FromContext(c); e == nil {
		return tx.WithContext(c)
	}
	return r.db.WithContext(c)
}

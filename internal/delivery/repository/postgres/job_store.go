package postgres

import (
	"context"
	"fmt"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type JobStore struct{ db *gorm.DB }

func NewJobStore(db *gorm.DB) *JobStore { return &JobStore{db: db} }

type jobRecord struct {
	ID, OrderID, IdempotencyKey uuid.UUID
	Provider, Status            string
	Attempts                    int
	AvailableAt                 time.Time
	LockedAt                    *time.Time
	LastError                   string
}

func (jobRecord) TableName() string { return "delivery_jobs" }

type orderRecord struct {
	ID          uuid.UUID
	Currency    string
	TotalAmount int64
}

func (orderRecord) TableName() string { return "orders" }

type detailsRecord struct {
	OrderID                                                                                                uuid.UUID
	RecipientName, RecipientPhone, CountryCode, PostalCode, City, Line1, Line2, LocalityID, ServicePointID string
}

func (detailsRecord) TableName() string { return "order_delivery_details" }

type itemRecord struct {
	VariantID *uuid.UUID
	Quantity  int
}

func (itemRecord) TableName() string { return "order_items" }
func (s *JobStore) Claim(ctx context.Context, now time.Time) (*domain.DispatchJob, error) {
	var claimed *domain.DispatchJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&jobRecord{}).Where("status = 'processing' AND locked_at < ?", now.Add(-5*time.Minute)).Updates(map[string]any{"status": "retrying", "available_at": now, "locked_at": nil}).Error; err != nil {
			return err
		}
		var job jobRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ? AND available_at <= ?", []string{"pending", "retrying"}, now).Order("created_at").First(&job).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		if err := tx.Model(&job).Updates(map[string]any{"status": "processing", "locked_at": now, "attempts": gorm.Expr("attempts + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error; err != nil {
			return err
		}
		var order orderRecord
		var details detailsRecord
		var items []itemRecord
		if err := tx.First(&order, "id = ?", job.OrderID).Error; err != nil {
			return err
		}
		if err := tx.First(&details, "order_id = ?", job.OrderID).Error; err != nil {
			return err
		}
		if err := tx.Where("order_id = ?", job.OrderID).Find(&items).Error; err != nil {
			return err
		}
		amount, err := money.New(order.TotalAmount, order.Currency)
		if err != nil {
			return err
		}
		shipmentItems := make([]domain.ShipmentItem, 0, len(items))
		for _, item := range items {
			if item.VariantID != nil {
				shipmentItems = append(shipmentItems, domain.ShipmentItem{VariantID: *item.VariantID, Quantity: item.Quantity})
			}
		}
		claimed = &domain.DispatchJob{ID: job.ID, OrderID: job.OrderID, Provider: job.Provider, IdempotencyKey: job.IdempotencyKey, Destination: domain.Address{RecipientName: details.RecipientName, RecipientPhone: details.RecipientPhone, CountryCode: details.CountryCode, PostalCode: details.PostalCode, City: details.City, Line1: details.Line1, Line2: details.Line2, LocalityID: details.LocalityID, ServicePointID: details.ServicePointID}, Items: shipmentItems, DeclaredValue: amount}
		return nil
	})
	return claimed, err
}
func (s *JobStore) Complete(ctx context.Context, id uuid.UUID, result domain.ShipmentResult) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job jobRecord
		if err := tx.First(&job, "id = ? AND status = 'processing'", id).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO deliveries (id, order_id, provider, tracking_number, status) VALUES (?, ?, ?, ?, 'created') ON CONFLICT (provider, tracking_number) WHERE tracking_number IS NOT NULL DO NOTHING`, uuid.New(), job.OrderID, job.Provider, result.TrackingNumber).Error; err != nil {
			return err
		}
		return tx.Model(&job).Updates(map[string]any{"status": "completed", "locked_at": nil, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
	})
}
func (s *JobStore) Retry(ctx context.Context, id uuid.UUID, cause error, availableAt time.Time) error {
	return s.transition(ctx, id, "retrying", cause, availableAt)
}
func (s *JobStore) Fail(ctx context.Context, id uuid.UUID, cause error) error {
	return s.transition(ctx, id, "failed", cause, time.Time{})
}
func (s *JobStore) transition(ctx context.Context, id uuid.UUID, status string, cause error, availableAt time.Time) error {
	values := map[string]any{"status": status, "locked_at": nil, "last_error": fmt.Sprint(cause), "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}
	if !availableAt.IsZero() {
		values["available_at"] = availableAt
	}
	return s.db.WithContext(ctx).Model(&jobRecord{}).Where("id = ? AND status = 'processing'", id).Updates(values).Error
}

var _ domain.JobStore = (*JobStore)(nil)
